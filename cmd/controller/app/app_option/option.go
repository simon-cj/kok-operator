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

package app_option

import (
	"github.com/spf13/pflag"
	"github.com/wtxue/kok-operator/pkg/option"
	"github.com/wtxue/kok-operator/pkg/provider/config"
)

type Options struct {
	Global   *option.GlobalManagerOption
	Ctrl     *option.ControllersManagerOption
	Provider *config.Config
}

// NewOptions creates a new Options with a default config.
func NewOptions() *Options {
	return &Options{
		Global:   option.DefaultGlobalManagetOption(),
		Ctrl:     option.DefaultControllersManagerOption(),
		Provider: config.NewDefaultConfig(),
	}
}

// AddFlags adds flags for a specific server to the specified FlagSet object.
func (o *Options) AddFlags(fs *pflag.FlagSet) {
	o.Global.AddFlags(fs)
	o.Ctrl.AddFlags(fs)
	o.Provider.AddFlags(fs)
}
