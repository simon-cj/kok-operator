package kubebin

import (
	"fmt"
	"strings"

	devopsv1 "github.com/wtxue/kok-operator/pkg/apis/devops/v1"
	"github.com/wtxue/kok-operator/pkg/constants"
	"github.com/wtxue/kok-operator/pkg/controllers/common"
	"github.com/wtxue/kok-operator/pkg/util/ssh"
)

const (
	kubeletService = `
[Unit]
Description=kubelet: The Kubernetes Node Agent
Documentation=https://kubernetes.io/docs/

[Service]
User=root
ExecStart=/usr/bin/kubelet
Restart=always
StartLimitInterval=0
RestartSec=10

[Install]
WantedBy=multi-user.target
`

	KubeletServiceRunConfig = `
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
EnvironmentFile=-/etc/sysconfig/kubelet
ExecStart=
ExecStart=/usr/bin/kubelet $KUBELET_KUBECONFIG_ARGS $KUBELET_CONFIG_ARGS $KUBELET_KUBEADM_ARGS $KUBELET_EXTRA_ARGS
`
)

func Install(ctx *common.ClusterContext, s ssh.Interface) error {
	// dir := "bin/linux/" // local debug config dir
	k8sDir := fmt.Sprintf("/k8s-%s/bin/", ctx.Cluster.Spec.Version)
	otherDir := "/k8s/bin/"
	if dir := constants.GetAnnotationKey(ctx.Cluster.Annotations, constants.ClusterAnnoLocalDebugDir); len(dir) > 0 {
		k8sDir = dir + k8sDir
		otherDir = dir + otherDir
	}

	var CopyList = []devopsv1.File{
		{
			Src: k8sDir + "kubectl",
			Dst: "/usr/local/bin/kubectl",
		},
		{
			Src: k8sDir + "kubeadm",
			Dst: "/usr/local/bin/kubeadm",
		},
		{
			Src: k8sDir + "kubelet",
			Dst: "/usr/bin/kubelet",
		},
		{
			Src: otherDir + "cni.tgz",
			Dst: "/opt/k8s/cni.tgz",
		},
	}

	for _, ls := range CopyList {
		if ok, err := s.Exist(ls.Dst); err == nil && ok {
			ctx.Info("file exist ignoring", "node", s.HostIP(), "dst", ls.Dst)
			continue
		}

		err := s.CopyFile(ls.Src, ls.Dst)
		if err != nil {
			ctx.Error(err, "CopyFile", "node", s.HostIP(), "src", ls.Src)
			return err
		}

		if strings.Contains(ls.Dst, "bin") {
			_, _, _, err = s.Execf("chmod a+x %s", ls.Dst)
			if err != nil {
				return err
			}
		}

		if strings.Contains(ls.Dst, "cni") {
			cmd := fmt.Sprintf("mkdir -p %s && tar -C %s -xzf /opt/k8s/cni.tgz", constants.CNIBinDir, constants.CNIBinDir)
			_, err := s.CombinedOutput(cmd)
			if err != nil {
				return err
			}
		}

		ctx.Info("copy success", "node", s.HostIP(), "dst", ls.Dst)
	}

	ctx.Info("write kubelet systemd unit file", "node", s.HostIP(), "dst", constants.KubeletSystemdUnitFilePath)
	err := s.WriteFile(strings.NewReader(kubeletService), constants.KubeletSystemdUnitFilePath)
	if err != nil {
		return err
	}

	ctx.Info("write kubelet systemd service run config", "node", s.HostIP(), "path", constants.KubeletServiceRunConfig)
	err = s.WriteFile(strings.NewReader(KubeletServiceRunConfig), constants.KubeletServiceRunConfig)
	if err != nil {
		return err
	}

	cmd := "mkdir -p /etc/kubernetes/manifests/ && systemctl enable kubelet && systemctl daemon-reload && systemctl restart kubelet"
	if _, stderr, exit, err := s.Execf(cmd); err != nil || exit != 0 {
		cmd = "journalctl --unit kubelet -n10 --no-pager"
		jStdout, _, jExit, jErr := s.Execf(cmd)
		if jErr != nil || jExit != 0 {
			return fmt.Errorf("exec %q:error %s", cmd, err)
		}

		ctx.Info("log", "cmd", cmd, "stdout", jStdout)
		return fmt.Errorf("Exec %s failed:exit %d:stderr %s:error %s:log:\n%s", cmd, exit, stderr, err, jStdout)
	}
	ctx.Info("exec successfully", "node", s.HostIP(), "cmd", cmd)

	cmd = fmt.Sprintf("kubectl completion bash > /etc/bash_completion.d/kubectl")
	_, err = s.CombinedOutput(cmd)
	if err != nil {
		return err
	}

	return nil
}
