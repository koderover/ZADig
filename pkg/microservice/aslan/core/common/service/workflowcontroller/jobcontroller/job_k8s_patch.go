/*
Copyright 2022 The KodeRover Authors.

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

package jobcontroller

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	crClient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/hashicorp/go-multierror"
	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	commonmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	kubeclient "github.com/koderover/zadig/pkg/shared/kube/client"
	"github.com/koderover/zadig/pkg/tool/kube/updater"
)

type K8sPatchJobCtl struct {
	job         *commonmodels.JobTask
	workflowCtx *commonmodels.WorkflowTaskCtx
	logger      *zap.SugaredLogger
	kubeClient  crClient.Client
	jobTaskSpec *commonmodels.JobTasK8sPatchSpec
	ack         func()
}

func NewK8sPatchJobCtl(job *commonmodels.JobTask, workflowCtx *commonmodels.WorkflowTaskCtx, ack func(), logger *zap.SugaredLogger) *K8sPatchJobCtl {
	jobTaskSpec := &commonmodels.JobTasK8sPatchSpec{}
	if err := commonmodels.IToi(job.Spec, jobTaskSpec); err != nil {
		logger.Error(err)
	}
	job.Spec = jobTaskSpec
	return &K8sPatchJobCtl{
		job:         job,
		workflowCtx: workflowCtx,
		logger:      logger,
		ack:         ack,
		jobTaskSpec: jobTaskSpec,
	}
}

func (c *K8sPatchJobCtl) Clean(ctx context.Context) {}

func (c *K8sPatchJobCtl) Run(ctx context.Context) {
	var err error
	c.kubeClient, err = kubeclient.GetKubeClient(config.HubServerAddress(), c.jobTaskSpec.ClusterID)
	if err != nil {
		msg := fmt.Sprintf("can't init k8s client: %v", err)
		logError(c.job, msg, c.logger)
		return
	}
	var wg sync.WaitGroup
	var errList *multierror.Error
	for _, patch := range c.jobTaskSpec.PatchItems {
		c.runPatch(patch, wg, errList)
	}
	wg.Wait()
	if err := errList.ErrorOrNil(); err != nil {
		msg := fmt.Sprintf("pacth resources error: %v", err)
		logError(c.job, msg, c.logger)
		return
	}
	c.job.Status = config.StatusPassed
}

func (c *K8sPatchJobCtl) runPatch(patchItem *commonmodels.PatchTaskItem, wg sync.WaitGroup, errList *multierror.Error) {
	wg.Add(1)
	defer wg.Done()
	var err error
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   patchItem.ResourceGroup,
		Version: patchItem.ResourceVersion,
		Kind:    patchItem.ResourceKind,
	})
	obj.SetName(patchItem.ResourceName)
	obj.SetNamespace(c.jobTaskSpec.Namespace)
	var patchBytes []byte
	var patchType types.PatchType
	switch patchItem.PatchStrategy {
	case "merge":
		resource := map[string]interface{}{}
		if err = yaml.Unmarshal([]byte(patchItem.PatchContent), &resource); err != nil {
			patchItem.Error = fmt.Sprintf("unmarshal yaml input error: %v", err)
			errList = multierror.Append(errList, err)
			return
		}
		patchBytes, err = json.Marshal(resource)
		if err != nil {
			patchItem.Error = fmt.Sprintf("marshal input into json error: %v", err)
			errList = multierror.Append(errList, err)
			return
		}
		patchType = types.MergePatchType
	case "strategic-merge":
		resource := map[string]interface{}{}
		if err := yaml.Unmarshal([]byte(patchItem.PatchContent), &resource); err != nil {
			patchItem.Error = fmt.Sprintf("unmarshal yaml input error: %v", err)
			errList = multierror.Append(errList, err)
			return
		}
		patchBytes, err = json.Marshal(resource)
		if err != nil {
			patchItem.Error = fmt.Sprintf("marshal input into json error: %v", err)
			errList = multierror.Append(errList, err)
			return
		}
		patchType = types.StrategicMergePatchType
	case "json":
		patchBytes = []byte(patchItem.PatchContent)
		patchType = types.JSONPatchType
	default:
		patchItem.Error = fmt.Sprintf("pacth strategy %s not supported", patchItem.PatchStrategy)
		errList = multierror.Append(errList, fmt.Errorf("pacth strategy %s not supported", patchItem.PatchStrategy))
	}

	if err = updater.PatchUnstructured(obj, patchBytes, patchType, c.kubeClient); err != nil {
		patchItem.Error = fmt.Sprintf("patch resoure error: %v", err)
		errList = multierror.Append(errList, err)
		return
	}
}
