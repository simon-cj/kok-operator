/*
Copyright 2020 wtxue.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package joinNode

import (
	"fmt"
	"os"

	"strings"

	"github.com/pkg/errors"
	kubeadmv1beta2 "github.com/wtxue/kube-on-kube-operator/pkg/apis/kubeadm/v1beta2"
	"github.com/wtxue/kube-on-kube-operator/pkg/constants"
	"github.com/wtxue/kube-on-kube-operator/pkg/controllers/common"
	"github.com/wtxue/kube-on-kube-operator/pkg/provider/certs"
	"github.com/wtxue/kube-on-kube-operator/pkg/provider/config"
	"github.com/wtxue/kube-on-kube-operator/pkg/provider/phases/kubeadm"
	"github.com/wtxue/kube-on-kube-operator/pkg/util/pkiutil"
	"github.com/wtxue/kube-on-kube-operator/pkg/util/ssh"
	"github.com/wtxue/kube-on-kube-operator/pkg/util/template"
	"k8s.io/klog"
)

func ApplyPodManifest(hostIP string, c *common.Cluster, cfg *config.Config, pathName string, podManifest string, fileMaps map[string]string) error {
	opt := &kubeadm.Option{
		HostIP:           hostIP,
		Images:           cfg.KubeAllImageFullName(constants.KubernetesAllImageName, c.Cluster.Spec.Version),
		EtcdPeerCluster:  kubeadm.BuildMasterEtcdPeerCluster(c),
		TokenClusterName: c.Cluster.Name,
	}

	serialized, err := template.ParseString(podManifest, opt)
	if err != nil {
		return err
	}

	fileMaps[pathName] = string(serialized)
	return nil
}

func BuildKubeletKubeconfig(hostIP string, c *common.Cluster, apiserver string, fileMaps map[string]string) error {
	cfgMaps, err := certs.CreateKubeConfigFiles(c.ClusterCredential.CAKey, c.ClusterCredential.CACert,
		apiserver, hostIP, c.Cluster.Name, pkiutil.KubeletKubeConfigFileName)
	if err != nil {
		klog.Errorf("create node: %s kubelet kubeconfg err: %+v", hostIP, err)
		return err
	}

	var kubeletConf []byte
	for _, v := range cfgMaps {
		data, err := certs.BuildKubeConfigByte(v)
		if err != nil {
			klog.Errorf("covert node: %s kubelet kubeconfg err: %+v", hostIP, err)
			return err
		}

		kubeletConf = data
		break
	}

	if kubeletConf == nil {
		return fmt.Errorf("node: %s can't build kubeletConf", hostIP)
	}

	fileMaps[constants.KubeletKubeConfigFileName] = string(kubeletConf)
	return nil
}

func JoinMasterNode(hostIP string, c *common.Cluster, cfg *config.Config, isMaster bool, fileMaps map[string]string) error {
	if !isMaster {
		fileMaps[constants.CACertName] = string(c.ClusterCredential.CACert)
		return nil
	}

	for pathName, va := range c.ClusterCredential.CertsBinaryData {
		fileMaps[pathName] = string(va)
	}

	for pathName, va := range c.ClusterCredential.KubeData {
		fileMaps[pathName] = va
	}

	for pathName, va := range c.ClusterCredential.ManifestsData {
		ApplyPodManifest(hostIP, c, cfg, pathName, va, fileMaps)
	}

	return nil
}

func JoinNodePhase(s ssh.Interface, cfg *config.Config, c *common.Cluster, apiserver string, isMaster bool) error {
	hostIP := s.HostIP()
	fileMaps := make(map[string]string)
	err := JoinMasterNode(hostIP, c, cfg, isMaster, fileMaps)
	if err != nil {
		return errors.Wrapf(err, "node: %s failed build misc file", hostIP)
	}

	err = BuildKubeletKubeconfig(hostIP, c, apiserver, fileMaps)
	if err != nil {
		return errors.Wrapf(err, "node: %s failed build kubelet file", hostIP)
	}

	nodeOpt := &kubeadmv1beta2.NodeRegistrationOptions{
		Name: hostIP,
	}
	flagsEnv := BuildKubeletDynamicEnvFile(cfg.Registry.Prefix, nodeOpt)
	fileMaps[constants.KubeletEnvFileName] = flagsEnv

	kubeletCfg := kubeadm.GetFullKubeletConfiguration(c)
	cfgYaml, err := KubeletMarshal(kubeletCfg)
	if err != nil {
		return errors.Wrapf(err, "node: %s failed marshal kubelet file", hostIP)
	}

	fileMaps[constants.KubeletConfigurationFileName] = string(cfgYaml)
	fileMaps[constants.KubeletServiceRunConfig] = kubeletEnvironmentTemplate

	for pathName, va := range fileMaps {
		klog.V(4).Infof("node: %s start write [%s] ...", hostIP, pathName)
		err = s.WriteFile(strings.NewReader(va), pathName)
		if err != nil {
			return errors.Wrapf(err, "node: %s failed to write for %s ", hostIP, pathName)
		}
	}

	klog.Infof("node: %s restart kubelet ... ", hostIP)
	cmd := fmt.Sprintf("mkdir -p /etc/kubernetes/manifests && systemctl enable kubelet && systemctl daemon-reload && systemctl restart kubelet")
	exit, err := s.ExecStream(cmd, os.Stdout, os.Stderr)
	if err != nil {
		klog.Errorf("%q %+v", exit, err)
		return err
	}
	return nil
}
