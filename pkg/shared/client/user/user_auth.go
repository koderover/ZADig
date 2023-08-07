package user

import (
	"errors"

	"github.com/koderover/zadig/pkg/tool/httpclient"
	"github.com/koderover/zadig/pkg/types"
)

type AuthorizedResources struct {
	IsSystemAdmin   bool                       `json:"is_system_admin"`
	ProjectAuthInfo map[string]*ProjectActions `json:"project_auth_info"`
	SystemActions   *SystemActions             `json:"system_actions"`
}

type ProjectActions struct {
	IsProjectAdmin    bool                      `json:"is_system_admin"`
	Workflow          *WorkflowActions          `json:"workflow"`
	Env               *EnvActions               `json:"env"`
	ProductionEnv     *ProductionEnvActions     `json:"production_env"`
	Service           *ServiceActions           `json:"service"`
	ProductionService *ProductionServiceActions `json:"production_service"`
	Build             *BuildActions             `json:"build"`
	Test              *TestActions              `json:"test"`
	Scanning          *ScanningActions          `json:"scanning"`
	Version           *VersionActions           `json:"version"`
}

type SystemActions struct {
	Project        *SystemProjectActions  `json:"project"`
	Template       *TemplateActions       `json:"template"`
	TestCenter     *TestCenterActions     `json:"test_center"`
	ReleaseCenter  *ReleaseCenterActions  `json:"release_center"`
	DeliveryCenter *DeliveryCenterActions `json:"delivery_center"`
	DataCenter     *DataCenterActions     `json:"data_center"`
}

type WorkflowActions struct {
	View    bool
	Create  bool
	Edit    bool
	Delete  bool
	Execute bool
	Debug   bool
}

type EnvActions struct {
	View   bool
	Create bool
	// 配置
	EditConfig bool
	// 管理服务实例
	ManagePods bool
	Delete     bool
	DebugPod   bool
	// 主机登录
	SSH bool
}

type ProductionEnvActions struct {
	View   bool
	Create bool
	// 配置
	EditConfig bool
	// 管理服务实例
	ManagePods bool
	Delete     bool
	DebugPod   bool
}

type ServiceActions struct {
	View   bool
	Create bool
	Edit   bool
	Delete bool
}

type ProductionServiceActions struct {
	View   bool
	Create bool
	Edit   bool
	Delete bool
}

type BuildActions struct {
	View   bool
	Create bool
	Edit   bool
	Delete bool
}

type TestActions struct {
	View    bool
	Create  bool
	Edit    bool
	Delete  bool
	Execute bool
}

type ScanningActions struct {
	View    bool
	Create  bool
	Edit    bool
	Delete  bool
	Execute bool
}

type VersionActions struct {
	View   bool
	Create bool
	Delete bool
}

type SystemProjectActions struct {
	Create bool
	Delete bool
}

type TemplateActions struct {
	Create bool
	View   bool
	Edit   bool
	Delete bool
}

type TestCenterActions struct {
	View bool
}

type ReleaseCenterActions struct {
	View bool
}

type DeliveryCenterActions struct {
	ViewArtifact bool
	ViewVersion  bool
}

type DataCenterActions struct {
	ViewOverView      bool
	ViewInsight       bool
	EditInsightConfig bool
}

func (c *Client) GetUserAuthInfo(uid string) (*AuthorizedResources, error) {
	url := "/auth-info"
	resp := &AuthorizedResources{}
	queries := make(map[string]string)
	queries["uid"] = uid

	_, err := c.Get(url, httpclient.SetQueryParams(queries), httpclient.SetResult(resp))
	return resp, err
}

func (c *Client) CheckUserAuthInfoForCollaborationMode(uid, projectKey, resource, resourceName, action string) (bool, error) {
	url := "/collaboration-permission"
	resp := &types.CheckCollaborationModePermissionResp{}

	queries := make(map[string]string)
	queries["uid"] = uid
	queries["project_key"] = projectKey
	queries["resource"] = resource
	queries["resource_name"] = resourceName
	queries["action"] = action

	_, err := c.Get(url, httpclient.SetQueryParams(queries), httpclient.SetResult(resp))
	if err != nil {
		return false, err
	}
	if len(resp.Error) > 0 {
		return resp.HasPermission, errors.New(resp.Error)
	}

	return resp.HasPermission, nil
}

func (c *Client) ListAuthorizedProjects(uid string) ([]string, bool, error) {
	url := "/authorized-projects"

	resp := &types.ListAuthorizedProjectResp{}

	queries := map[string]string{
		"uid": uid,
	}

	_, err := c.Get(url, httpclient.SetQueryParams(queries), httpclient.SetResult(resp))
	if err != nil {
		return []string{}, false, err
	}
	if len(resp.Error) > 0 {
		return []string{}, false, errors.New(resp.Error)
	}
	return resp.ProjectList, resp.Found, nil
}

func (c *Client) ListAuthorizedProjectsByResourceAndVerb(uid, resource, verb string) ([]string, bool, error) {
	url := "/authorized-projects"

	resp := &types.ListAuthorizedProjectResp{}

	queries := map[string]string{
		"uid":      uid,
		"resource": resource,
		"verb":     verb,
	}

	_, err := c.Get(url, httpclient.SetQueryParams(queries), httpclient.SetResult(resp))
	if err != nil {
		return []string{}, false, err
	}
	if len(resp.Error) > 0 {
		return []string{}, false, errors.New(resp.Error)
	}
	return resp.ProjectList, resp.Found, nil
}

func (c *Client) ListAuthorizedWorkflows(uid, projectKey string) ([]string, []string, error) {
	url := "/authorized-workflows"

	resp := &types.ListAuthorizedWorkflowsResp{}

	queries := map[string]string{
		"uid":         uid,
		"project_key": projectKey,
	}

	_, err := c.Get(url, httpclient.SetQueryParams(queries), httpclient.SetResult(resp))
	if err != nil {
		return []string{}, []string{}, err
	}
	if len(resp.Error) > 0 {
		return []string{}, []string{}, errors.New(resp.Error)
	}
	return resp.WorkflowList, resp.CustomWorkflowList, nil
}

func (c *Client) ListCollaborationEnvironmentsPermission(uid, projectKey string) (*types.CollaborationEnvPermission, error) {
	url := "/authorized-envs"

	resp := &types.CollaborationEnvPermission{}

	queries := map[string]string{
		"uid":         uid,
		"project_key": projectKey,
	}

	_, err := c.Get(url, httpclient.SetQueryParams(queries), httpclient.SetResult(resp))
	if err != nil {
		return nil, err
	}
	if len(resp.Error) > 0 {
		return nil, errors.New(resp.Error)
	}
	return resp, nil
}

func (c *Client) CheckPermissionGivenByCollaborationMode(uid, projectKey, resource, action string) (bool, error) {
	url := "/collaboration-action"
	resp := &types.CheckCollaborationModePermissionResp{}

	queries := make(map[string]string)
	queries["uid"] = uid
	queries["project_key"] = projectKey
	queries["resource"] = resource
	queries["action"] = action

	_, err := c.Get(url, httpclient.SetQueryParams(queries), httpclient.SetResult(resp))
	if err != nil {
		return false, err
	}
	if len(resp.Error) > 0 {
		return resp.HasPermission, errors.New(resp.Error)
	}

	return resp.HasPermission, nil
}
