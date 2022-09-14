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
	"fmt"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/labels"
	crClient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	commonmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	"github.com/koderover/zadig/pkg/setting"
	kubeclient "github.com/koderover/zadig/pkg/shared/kube/client"
	"github.com/koderover/zadig/pkg/shared/kube/wrapper"
	krkubeclient "github.com/koderover/zadig/pkg/tool/kube/client"
	"github.com/koderover/zadig/pkg/tool/kube/getter"
	"github.com/koderover/zadig/pkg/tool/kube/updater"
)

type BlueGreenDeployJobCtl struct {
	job         *commonmodels.JobTask
	workflowCtx *commonmodels.WorkflowTaskCtx
	logger      *zap.SugaredLogger
	kubeClient  crClient.Client
	jobTaskSpec *commonmodels.JobTaskBlueGreenDeploySpec
	ack         func()
}

func NewBlueGreenDeployJobCtl(job *commonmodels.JobTask, workflowCtx *commonmodels.WorkflowTaskCtx, ack func(), logger *zap.SugaredLogger) *BlueGreenDeployJobCtl {
	jobTaskSpec := &commonmodels.JobTaskBlueGreenDeploySpec{}
	if err := commonmodels.IToi(job.Spec, jobTaskSpec); err != nil {
		logger.Error(err)
	}
	if jobTaskSpec.Events == nil {
		jobTaskSpec.Events = &commonmodels.Events{}
	}
	job.Spec = jobTaskSpec
	return &BlueGreenDeployJobCtl{
		job:         job,
		workflowCtx: workflowCtx,
		logger:      logger,
		ack:         ack,
		jobTaskSpec: jobTaskSpec,
	}
}

func (c *BlueGreenDeployJobCtl) Clean(ctx context.Context) {}

func (c *BlueGreenDeployJobCtl) Run(ctx context.Context) {
	if err := c.run(ctx); err != nil {
		return
	}
	c.wait(ctx)
}

func (c *BlueGreenDeployJobCtl) run(ctx context.Context) error {
	var err error
	if c.jobTaskSpec.ClusterID != "" {
		c.kubeClient, err = kubeclient.GetKubeClient(config.HubServerAddress(), c.jobTaskSpec.ClusterID)
		if err != nil {
			msg := fmt.Sprintf("can't init k8s client: %v", err)
			c.logger.Error(msg)
			c.job.Status = config.StatusFailed
			c.job.Error = msg
			c.jobTaskSpec.Events.Error(msg)
			return errors.New(msg)
		}
	} else {
		c.kubeClient = krkubeclient.Client()
	}

	service, exist, err := getter.GetService(c.jobTaskSpec.Namespace, c.jobTaskSpec.K8sServiceName, c.kubeClient)
	if err != nil || !exist {
		msg := fmt.Sprintf("service not found: %v", err)
		c.logger.Error(msg)
		c.job.Status = config.StatusFailed
		c.job.Error = msg
		c.jobTaskSpec.Events.Error(msg)
		return errors.New(msg)
	}
	selector := labels.Set(service.Spec.Selector).AsSelector()

	deployment, exist, err := getter.GetDeployment(c.jobTaskSpec.Namespace, c.jobTaskSpec.WorkloadName, c.kubeClient)
	if err != nil || !exist {
		msg := fmt.Sprintf("deployment not found: %v", err)
		c.logger.Error(msg)
		c.job.Status = config.StatusFailed
		c.job.Error = msg
		c.jobTaskSpec.Events.Error(msg)
		return errors.New(msg)
	}
	// if label not exist, we think this was the first time to deploy, so we need to add the label
	if previousLabel, ok := deployment.ObjectMeta.Labels[config.BlueGreenVerionLabelName]; !ok {
		c.jobTaskSpec.FirstDeploy = true
		c.jobTaskSpec.Events.Info(fmt.Sprintf("deployment %s was the first time blue-green deploy by zadig", c.jobTaskSpec.WorkloadName))
		c.ack()

		pods, err := getter.ListPods(c.jobTaskSpec.Namespace, selector, c.kubeClient)
		if err != nil {
			msg := fmt.Sprintf("list pods error: %v", err)
			c.logger.Error(msg)
			c.job.Status = config.StatusFailed
			c.job.Error = msg
			c.jobTaskSpec.Events.Error(msg)
			return errors.New(msg)
		}
		for _, pod := range pods {
			addlabelPatch := fmt.Sprintf(`[{"op":"add","path":"/metadata/labels/%s","value":"%s" }]`, config.BlueGreenVerionLabelName, config.OriginVersion)
			if err := updater.PatchPod(c.jobTaskSpec.Namespace, pod.Name, []byte(addlabelPatch), c.kubeClient); err != nil {
				msg := fmt.Sprintf("add origin label to pod error: %v", err)
				c.logger.Error(msg)
				c.job.Status = config.StatusFailed
				c.job.Error = msg
				c.jobTaskSpec.Events.Error(msg)
				return errors.New(msg)
			}
		}
		c.jobTaskSpec.Events.Info("add origin label to pods")
		c.ack()
		service.Spec.Selector[config.BlueGreenVerionLabelName] = config.OriginVersion
		if err := updater.CreateOrPatchService(service, c.kubeClient); err != nil {
			msg := fmt.Sprintf("add origin label selector to serivce error: %v", err)
			c.logger.Error(msg)
			c.job.Status = config.StatusFailed
			c.job.Error = msg
			c.jobTaskSpec.Events.Error(msg)
			return errors.New(msg)
		}
		c.jobTaskSpec.Events.Info("add origin label selector to service")
		c.ack()
	} else {
		// ensure service have the label selector match deployments.
		if _, ok := service.Spec.Selector[config.BlueGreenVerionLabelName]; !ok {
			service.Spec.Selector[config.BlueGreenVerionLabelName] = previousLabel
			if err := updater.CreateOrPatchService(service, c.kubeClient); err != nil {
				msg := fmt.Sprintf("add label selector to serivce error: %v", err)
				c.logger.Error(msg)
				c.job.Status = config.StatusFailed
				c.job.Error = msg
				c.jobTaskSpec.Events.Error(msg)
				return errors.New(msg)
			}
			c.jobTaskSpec.Events.Info("add label selector to service")
			c.ack()
		}
	}
	blueService := service.DeepCopy()
	blueService.Name = c.jobTaskSpec.BlueK8sServiceName
	c.jobTaskSpec.BlueK8sServiceName = blueService.Name
	blueService.Spec.Selector[config.BlueGreenVerionLabelName] = c.jobTaskSpec.Version
	if err := updater.CreateOrPatchService(blueService, c.kubeClient); err != nil {
		msg := fmt.Sprintf("create blue serivce error: %v", err)
		c.logger.Error(msg)
		c.job.Status = config.StatusFailed
		c.job.Error = msg
		c.jobTaskSpec.Events.Error(msg)
		return errors.New(msg)
	}
	c.jobTaskSpec.Events.Info("blue service created")
	c.ack()
	blueDeployment := deployment.DeepCopy()
	blueDeployment.Name = c.jobTaskSpec.BlueWorkloadName
	blueDeployment.Labels[config.BlueGreenVerionLabelName] = c.jobTaskSpec.Version
	blueDeployment.Spec.Selector.MatchLabels[config.BlueGreenVerionLabelName] = c.jobTaskSpec.Version
	blueDeployment.Spec.Template.Labels[config.BlueGreenVerionLabelName] = c.jobTaskSpec.Version
	blueDeployment.ObjectMeta.ResourceVersion = ""
	if err := updater.CreateOrPatchDeployment(blueDeployment, c.kubeClient); err != nil {
		msg := fmt.Sprintf("create blue deployment error: %v", err)
		c.logger.Error(msg)
		c.job.Status = config.StatusFailed
		c.job.Error = msg
		c.jobTaskSpec.Events.Error(msg)
		return errors.New(msg)
	}
	c.jobTaskSpec.Events.Info("blue deployment created")
	c.ack()
	return nil
}

func (c *BlueGreenDeployJobCtl) wait(ctx context.Context) {
	timeout := time.After(time.Duration(c.timeout()) * time.Second)
	for {
		select {
		case <-ctx.Done():
			c.job.Status = config.StatusCancelled
			return

		case <-timeout:
			c.job.Status = config.StatusTimeout
			msg := fmt.Sprintf("timeout waiting for the blue deployment %s to run", c.jobTaskSpec.BlueWorkloadName)
			c.jobTaskSpec.Events.Info(msg)
			return

		default:
			time.Sleep(time.Second * 2)
			d, found, err := getter.GetDeployment(c.jobTaskSpec.Namespace, c.jobTaskSpec.BlueWorkloadName, c.kubeClient)
			if err != nil || !found {
				c.logger.Errorf(
					"failed to check deployment ready status %s/%s - %v",
					c.jobTaskSpec.Namespace,
					c.jobTaskSpec.BlueWorkloadName,
					err,
				)
			} else {
				if wrapper.Deployment(d).Ready() {
					c.job.Status = config.StatusPassed
					msg := fmt.Sprintf("canary deployment %s create successfully", c.jobTaskSpec.BlueWorkloadName)
					c.jobTaskSpec.Events.Info(msg)
					return
				}
			}
		}
	}
}

func (c *BlueGreenDeployJobCtl) timeout() int64 {
	if c.jobTaskSpec.DeployTimeout == 0 {
		c.jobTaskSpec.DeployTimeout = setting.DeployTimeout
	} else {
		c.jobTaskSpec.DeployTimeout = c.jobTaskSpec.DeployTimeout * 60
	}
	return c.jobTaskSpec.DeployTimeout
}
