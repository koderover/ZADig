/*
Copyright 2021 The KodeRover Authors.

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

package kube

import (
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/koderover/zadig/pkg/internal/kube/wrapper"
	"github.com/koderover/zadig/pkg/setting"
	"github.com/koderover/zadig/pkg/tool/kube/getter"
)

// TODO: improve this function
func GetSelectedPodsInfo(ns string, selector labels.Selector, kubeClient client.Client, log *zap.SugaredLogger) (string, string, []string) {
	pods, err := getter.ListPods(ns, selector, kubeClient)
	if err != nil {
		return setting.PodError, setting.PodNotReady, nil
	}

	if len(pods) == 0 {
		return setting.PodNonStarted, setting.PodNotReady, nil
	}

	imageSet := sets.String{}
	for _, pod := range pods {
		ipod := wrapper.Pod(pod)
		imageSet.Insert(ipod.Containers()...)
	}
	images := imageSet.List()

	ready := setting.PodReady

	succeededPods := 0
	for _, pod := range pods {
		iPod := wrapper.Pod(pod)

		// skip if pod is a succeeded job
		if iPod.Succeeded() {
			succeededPods++
			continue
		}

		// 如果服务中任意一个pod不在running状态，返回unstable
		if !iPod.Ready() {
			return setting.PodUnstable, setting.PodNotReady, images
		}
	}

	// return "Succeeded, Completed" if all pods are succeeded Job pods
	if len(pods) == succeededPods {
		return string(corev1.PodSucceeded), setting.JobReady, images
	}

	return setting.PodRunning, ready, images
}
