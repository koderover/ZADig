/*
Copyright 2023 The KodeRover Authors.

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

package types

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/koderover/zadig/pkg/setting"
)

type ZadigJobTask struct {
	ID            string   `bson:"_id"                    json:"id"`
	ProjectName   string   `bson:"project_name"           json:"project_name"`
	WorkflowName  string   `bson:"workflow_name"          json:"workflow_name"`
	TaskID        int64    `bson:"task_id"                json:"task_id"`
	JobOriginName string   `bson:"job_origin_name"        json:"job_origin_name"`
	JobName       string   `bson:"job_name"               json:"job_name"`
	Status        string   `bson:"status"                 json:"status"`
	VMID          string   `bson:"vm_id"                  json:"vm_id"`
	StartTime     int64    `bson:"start_time"             json:"start_time"`
	EndTime       int64    `bson:"end_time"               json:"end_time"`
	Error         string   `bson:"error"                  json:"error"`
	VMLabels      []string `bson:"vm_labels"              json:"vm_labels"`
	VMName        []string `bson:"vm_name"                json:"vm_name"`
	JobCtx        string   `bson:"job_ctx"                json:"job_ctx"`
}

type Step struct {
	Name      string      `json:"name"`
	StepType  string      `json:"type"`
	Onfailure bool        `json:"on_failure"`
	Spec      interface{} `json:"spec"`
}

type JobContext struct {
	Name string `yaml:"name"`
	// Workspace 容器工作目录 [必填]
	Workspace string `yaml:"workspace"`
	Proxy     *Proxy `yaml:"proxy"`
	// Envs 用户注入环境变量, 包括安装脚本环境变量 [optional]
	Envs EnvVar `yaml:"envs"`
	// SecretEnvs 用户注入敏感信息环境变量, value不能在stdout stderr中输出 [optional]
	SecretEnvs EnvVar `yaml:"secret_envs"`
	// WorkflowName
	WorkflowName string `yaml:"workflow_name"`
	// TaskID
	TaskID int64 `yaml:"task_id"`
	// Paths 执行脚本Path
	Paths string `yaml:"paths"`
	// ConfigMapName save the name of the configmap in which the jobContext resides
	ConfigMapName string `yaml:"config_map_name"`

	Steps   []*StepTask `yaml:"steps"`
	Outputs []string    `yaml:"outputs"`
}

func (j *JobContext) Decode(job string) error {
	if err := yaml.Unmarshal([]byte(job), j); err != nil {
		return err
	}
	return nil
}

type StepTask struct {
	Name      string `bson:"name"           json:"name"         yaml:"name"`
	JobName   string `bson:"job_name"       json:"job_name"     yaml:"job_name"`
	Error     string `bson:"error"          json:"error"        yaml:"error"`
	StepType  string `bson:"type"           json:"type"         yaml:"type"`
	Onfailure bool   `bson:"on_failure"     json:"on_failure"   yaml:"on_failure"`
	// step input params,differ form steps
	Spec interface{} `bson:"spec"           json:"spec"   yaml:"spec"`
	// step output results,like testing results,differ form steps
	Result interface{} `bson:"result"         json:"result"  yaml:"result"`
}

// Proxy 代理配置信息
type Proxy struct {
	Type                   string `yaml:"type"`
	Address                string `yaml:"address"`
	Port                   int    `yaml:"port"`
	NeedPassword           bool   `yaml:"need_password"`
	Username               string `yaml:"username"`
	Password               string `yaml:"password"`
	EnableRepoProxy        bool   `yaml:"enable_repo_proxy"`
	EnableApplicationProxy bool   `yaml:"enable_application_proxy"`
}

type EnvVar []string

type ReportJobParameters struct {
	Seq       int    `json:"seq"`
	Token     string `json:"token"`
	JobID     string `json:"job_id"`
	JobStatus string `json:"job_status"`
	JobLog    string `json:"job_log"`
	JobError  string `json:"job_error"`
	JobOutput []byte `json:"job_output"`
}

const (
	JobOutputDir       = "zadig/results/"
	JobTerminationFile = "/zadig/termination"
)

func GetJobOutputKey(key, outputName string) string {
	return fmt.Sprintf(setting.RenderValueTemplate, strings.Join([]string{"job", key, "output", outputName}, "."))
}

type ReportAgentJobResp struct {
	JobID     string `json:"job_id"`
	JobStatus string `json:"job_status"`
}
