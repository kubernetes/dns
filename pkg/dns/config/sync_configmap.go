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

package config

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"time"

	"github.com/golang/glog"
)

// NewConfigMapSync returns a Sync that watches a config map in the API
func NewConfigMapSync(client kubernetes.Interface, ns string, name string) Sync {
	syncSource := &kubeAPISyncSource{
		ns:      ns,
		name:    name,
		client:  client,
		channel: make(chan syncResult),
	}

	listWatch := cache.NewListWatchFromClient(
		syncSource.client.CoreV1().RESTClient(),
		"configmaps",
		ns,
		fields.Everything())

	store, controller := cache.NewInformer(
		listWatch,
		&v1.ConfigMap{},
		time.Duration(0),
		cache.ResourceEventHandlerFuncs{
			AddFunc:    syncSource.onAdd,
			DeleteFunc: syncSource.onDelete,
			UpdateFunc: syncSource.onUpdate,
		})

	syncSource.store = store
	syncSource.controller = controller

	return newSync(syncSource)
}

type kubeAPISyncSource struct {
	ns   string
	name string

	client     kubernetes.Interface
	store      cache.Store
	controller cache.Controller

	channel chan syncResult
}

func (syncSource *kubeAPISyncSource) Once() (syncResult, error) {
	cm, err := syncSource.client.CoreV1().ConfigMaps(syncSource.ns).Get(syncSource.name, metav1.GetOptions{})
	if err != nil {
		glog.Errorf("Error getting ConfigMap %v:%v err: %v", syncSource.ns, syncSource.name, err)
		return syncResult{}, err
	}
	return syncResult{Version: cm.ResourceVersion, Data: cm.Data}, nil
}

func (syncSource *kubeAPISyncSource) Periodic() <-chan syncResult {
	go syncSource.controller.Run(wait.NeverStop)
	return syncSource.channel
}

func (syncSource *kubeAPISyncSource) toConfigMap(obj interface{}) *v1.ConfigMap {
	cm, ok := obj.(*v1.ConfigMap)
	if !ok {
		glog.Fatalf("Expected ConfigMap, got %T", obj)
	}
	return cm
}

func (syncSource *kubeAPISyncSource) onAdd(obj interface{}) {
	cm := syncSource.toConfigMap(obj)
	glog.V(2).Infof("ConfigMap %s:%s was created", syncSource.ns, syncSource.name)
	syncSource.channel <- syncResult{Version: cm.ResourceVersion, Data: cm.Data}
}

func (syncSource *kubeAPISyncSource) onDelete(_ interface{}) {
	glog.V(2).Infof("ConfigMap %s:%s was deleted, reverting to default configuration", syncSource.ns, syncSource.name)
	syncSource.channel <- syncResult{Version: "", Data: nil}
}

func (syncSource *kubeAPISyncSource) onUpdate(_, obj interface{}) {
	cm := syncSource.toConfigMap(obj)
	glog.V(2).Infof("ConfigMap %s:%s was updated", syncSource.ns, syncSource.name)
	syncSource.channel <- syncResult{Version: cm.ResourceVersion, Data: cm.Data}
}
