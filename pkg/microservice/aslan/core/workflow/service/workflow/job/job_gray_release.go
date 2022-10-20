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

package job

import (
	"fmt"
	"math"

	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	commonmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	commonrepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	kubeclient "github.com/koderover/zadig/pkg/shared/kube/client"
	"github.com/koderover/zadig/pkg/tool/kube/getter"
)

const (
	GrayDeploymentSuffix = "-zadig-gray"
)

type GrayReleaseJob struct {
	job      *commonmodels.Job
	workflow *commonmodels.WorkflowV4
	spec     *commonmodels.GrayReleaseJobSpec
}

func (j *GrayReleaseJob) Instantiate() error {
	j.spec = &commonmodels.GrayReleaseJobSpec{}
	if err := commonmodels.IToiYaml(j.job.Spec, j.spec); err != nil {
		return err
	}
	j.job.Spec = j.spec
	return nil
}

func (j *GrayReleaseJob) SetPreset() error {
	j.spec = &commonmodels.GrayReleaseJobSpec{}
	if err := commonmodels.IToi(j.job.Spec, j.spec); err != nil {
		return err
	}
	j.job.Spec = j.spec
	return nil
}

func (j *GrayReleaseJob) MergeArgs(args *commonmodels.Job) error {
	if j.job.Name == args.Name && j.job.JobType == args.JobType {
		j.spec = &commonmodels.GrayReleaseJobSpec{}
		if err := commonmodels.IToi(j.job.Spec, j.spec); err != nil {
			return err
		}
		j.job.Spec = j.spec
		argsSpec := &commonmodels.GrayReleaseJobSpec{}
		if err := commonmodels.IToi(args.Spec, argsSpec); err != nil {
			return err
		}
		j.spec.Targets = argsSpec.Targets
		j.job.Spec = j.spec
	}
	return nil
}

func (j *GrayReleaseJob) ToJobs(taskID int64) ([]*commonmodels.JobTask, error) {
	// logger := log.SugaredLogger()
	resp := []*commonmodels.JobTask{}
	j.spec = &commonmodels.GrayReleaseJobSpec{}
	if err := commonmodels.IToi(j.job.Spec, j.spec); err != nil {
		return resp, err
	}
	// if from job is empty, it was the first deploy Job.
	firstJob := false
	if j.spec.FromJob != "" {
		if j.spec.GrayScale > 100 {
			return resp, fmt.Errorf("release job: %s release percentage cannot largger than 100%", j.job.Name)
		}
		found := false
		for _, stage := range j.workflow.Stages {
			for _, job := range stage.Jobs {
				if job.Name != j.spec.FromJob || job.JobType != config.JobK8sGrayRelease {
					continue
				}
				found = true
				fromJobSpec := &commonmodels.GrayReleaseJobSpec{}
				if err := commonmodels.IToi(job.Spec, fromJobSpec); err != nil {
					return resp, err
				}
				j.spec.ClusterID = fromJobSpec.ClusterID
				j.spec.Namespace = fromJobSpec.Namespace
				j.spec.DockerRegistryID = fromJobSpec.DockerRegistryID
				j.spec.Targets = fromJobSpec.Targets
			}
		}
		if !found {
			return resp, fmt.Errorf("gray release job: %s not found", j.spec.FromJob)
		}
	} else {
		firstJob = true
		if j.spec.GrayScale >= 100 {
			return resp, fmt.Errorf("the first release job: %s cannot be released in full", j.job.Name)
		}
		kubeClient, err := kubeclient.GetKubeClient(config.HubServerAddress(), j.spec.ClusterID)
		if err != nil {
			return resp, fmt.Errorf("Failed to get kube client, err: %v", err)
		}
		for _, target := range j.spec.Targets {
			deployment, found, err := getter.GetDeployment(j.spec.Namespace, target.WorkloadName, kubeClient)
			if err != nil || !found {
				return resp, fmt.Errorf("deployment %s not found in namespace: %s", target.WorkloadName, j.spec.Namespace)
			}
			target.Replica = int(*deployment.Spec.Replicas)
		}
	}

	cluster, err := commonrepo.NewK8SClusterColl().Get(j.spec.ClusterID)
	if err != nil {
		return resp, fmt.Errorf("cluster id: %s not found", j.spec.ClusterID)
	}

	for _, target := range j.spec.Targets {
		grayReplica := math.Ceil(float64(*&target.Replica) * (float64(j.spec.GrayScale) / 100))
		jobTask := &commonmodels.JobTask{
			Name:    j.job.Name,
			JobType: string(config.JobK8sGrayRelease),
			Spec: &commonmodels.JobTaskGrayReleaseSpec{
				ClusterID:        j.spec.ClusterID,
				ClusterName:      cluster.Name,
				Namespace:        j.spec.Namespace,
				WorkloadType:     target.WorkloadType,
				WorkloadName:     target.WorkloadName,
				ContainerName:    target.ContainerName,
				FirstJob:         firstJob,
				GrayWorkloadName: target.WorkloadName + GrayDeploymentSuffix,
				Image:            target.Image,
				DeployTimeout:    j.spec.DeployTimeout,
				GrayScale:        j.spec.GrayScale,
				TotalReplica:     target.Replica,
				GrayReplica:      int(grayReplica),
			},
		}
		resp = append(resp, jobTask)
	}
	j.job.Spec = j.spec
	return resp, nil
}

func (j *GrayReleaseJob) LintJob() error {
	// TODO
	return nil
}
