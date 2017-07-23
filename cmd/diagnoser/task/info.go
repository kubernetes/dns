/*
Copyright 2017 The Kubernetes Authors.

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

package task

import (
	"github.com/golang/glog"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/dns/cmd/diagnoser/flags"
)

type info struct{}

func init() {
	register(&info{})
}

func (i *info) Run(opt *flags.Options, cs v1.CoreV1Interface) error {
	if !opt.RunInfo {
		return nil
	}

	dnsPods, err := cs.Pods("kube-system").List(meta_v1.ListOptions{
		LabelSelector: `k8s-app=kube-dns`})
	if err != nil {
		return err
	}

	glog.Infof("Total DNS pods: %d", len(dnsPods.Items))

	return nil
}
