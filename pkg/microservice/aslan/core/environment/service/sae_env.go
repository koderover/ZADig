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

package service

import (
	"fmt"
	"strings"

	sae20190506 "github.com/alibabacloud-go/sae-20190506/client"
	"github.com/alibabacloud-go/tea/tea"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/koderover/zadig/v2/pkg/microservice/aslan/core/common/repository/models"
	"github.com/koderover/zadig/v2/pkg/microservice/aslan/core/common/repository/mongodb"
	commonrepo "github.com/koderover/zadig/v2/pkg/microservice/aslan/core/common/repository/mongodb"
	commonservice "github.com/koderover/zadig/v2/pkg/microservice/aslan/core/common/service"
	"github.com/koderover/zadig/v2/pkg/microservice/aslan/core/common/service/sae"
	"github.com/koderover/zadig/v2/pkg/setting"
	e "github.com/koderover/zadig/v2/pkg/tool/errors"
	"github.com/pkg/errors"
)

func ListSAEEnvs(userID, projectName string, log *zap.SugaredLogger) ([]*EnvResp, error) {
	envs, err := commonrepo.NewSAEEnvColl().List(&commonrepo.SAEEnvListOptions{
		ProjectName:         projectName,
		IsSortByProductName: true,
	})
	if err != nil {
		log.Errorf("Failed to list sae envs, err: %s", err)
		return nil, e.ErrListEnvs.AddDesc(err.Error())
	}

	var res []*EnvResp
	list, err := commonservice.ListFavorites(&mongodb.FavoriteArgs{
		UserID:      userID,
		ProductName: projectName,
		Type:        commonservice.FavoriteTypeEnv,
	})
	if err != nil {
		return nil, errors.Wrap(err, "list favorite environments")
	}
	// add personal favorite data in response
	favSet := sets.NewString(func() []string {
		var nameList []string
		for _, fav := range list {
			nameList = append(nameList, fav.Name)
		}
		return nameList
	}()...)

	for _, env := range envs {
		res = append(res, &EnvResp{
			ProjectName: projectName,
			Name:        env.EnvName,
			UpdateTime:  env.UpdateTime,
			UpdateBy:    env.UpdateBy,
			Namespace:   env.NamespaceID,
			IsFavorite:  favSet.Has(env.EnvName),
		})
	}

	return res, nil
}

func GetSAEEnv(username, envName, productName string, log *zap.SugaredLogger) (*models.SAEEnv, error) {
	opt := &commonrepo.SAEEnvFindOptions{ProjectName: productName, EnvName: envName}
	env, err := commonrepo.NewSAEEnvColl().Find(opt)
	if err != nil {
		log.Errorf("[User:%s][EnvName:%s][Product:%s] Product.FindByOwner error: %s", username, envName, productName, err)
		return nil, e.ErrGetEnv
	}

	return env, nil
}

func CreateSAEEnv(username string, env *models.SAEEnv, isCreateEnv bool, log *zap.SugaredLogger) error {
	saeModel, err := commonrepo.NewSAEColl().FindDefault()
	if err != nil {
		err = fmt.Errorf("Failed to find default sae, err: %s", err)
		log.Error(err)
		return e.ErrCreateEnv.AddErr(err)
	}
	saeClient, err := sae.NewClient(saeModel, env.RegionID)
	if err != nil {
		err = fmt.Errorf("Failed to create sae client, err: %s", err)
		log.Error(err)
		return e.ErrCreateEnv.AddErr(err)
	}

	// remove tags
	if !isCreateEnv {
		addedApps := sets.NewString()
		for _, app := range env.Applications {
			addedApps.Insert(app.AppID)
		}

		currentPage := int32(1)
		pageSize := int32(1000)
		for {
			resp, err := ListSAEApps(env.RegionID, env.NamespaceID, env.ProjectName, env.EnvName, "", false, currentPage, pageSize, log)
			if err != nil {
				err = fmt.Errorf("Failed to list sae apps, err: %s", err)
				log.Error(err)
				return e.ErrDeleteEnv.AddErr(err)
			}

			resourceIds := "["
			for _, app := range resp.Applications {
				projectMatched := false
				envMatched := false
				for _, tag := range app.Tags {
					if tag.Key == setting.SAEZadigProjectTagKey && tag.Value == env.ProjectName {
						projectMatched = true
					}
					if tag.Key == setting.SAEZadigEnvTagKey && tag.Value == env.EnvName {
						envMatched = true
					}
				}
				if projectMatched && envMatched && !addedApps.Has(app.AppID) {
					resourceIds += fmt.Sprintf(`"%s",`, app.AppID)
				}
			}
			resourceIds = strings.TrimSuffix(resourceIds, ",") + "]"
			saeRequest := &sae20190506.UntagResourcesRequest{
				RegionId:     tea.String(env.RegionID),
				ResourceType: tea.String("application"),
				TagKeys:      tea.String(fmt.Sprintf(`["%s","%s"]`, setting.SAEZadigProjectTagKey, setting.SAEZadigEnvTagKey)),
				ResourceIds:  tea.String(resourceIds),
			}
			saeResp, err := saeClient.UntagResources(saeRequest)
			if err != nil {
				err = fmt.Errorf("Failed to un tag resources, err: %s", err)
				log.Error(err)
				return e.ErrDeleteEnv.AddErr(err)
			}
			if !tea.BoolValue(saeResp.Body.Success) {
				err = fmt.Errorf("Failed to un tag resources, statusCode: %d, code: %s, errCode: %s, message: %s", tea.Int32Value(saeResp.StatusCode), tea.ToString(saeResp.Body.Code), tea.ToString(saeResp.Body.ErrorCode), tea.ToString(saeResp.Body.Message))
				log.Error(err)
				return e.ErrDeleteEnv.AddErr(err)
			}

			if currentPage*pageSize >= resp.TotalSize {
				break
			}
			currentPage++
			pageSize += 1000
		}
	}

	// add tags
	resourceIds := "["
	for _, app := range env.Applications {
		resourceIds += fmt.Sprintf(`"%s",`, app.AppID)
	}
	resourceIds = strings.TrimSuffix(resourceIds, ",") + "]"
	saeRequest := &sae20190506.TagResourcesRequest{
		RegionId:     tea.String(env.RegionID),
		ResourceType: tea.String("application"),
		Tags:         tea.String(fmt.Sprintf(`[{"Key":"%s","Value":"%s"}, {"Key":"%s","Value":"%s"}]`, setting.SAEZadigProjectTagKey, env.ProjectName, setting.SAEZadigEnvTagKey, env.EnvName)),
		ResourceIds:  tea.String(resourceIds),
	}
	saeResp, err := saeClient.TagResources(saeRequest)
	if err != nil {
		err = fmt.Errorf("Failed to tag resources, err: %s", err)
		log.Error(err)
		return e.ErrCreateEnv.AddErr(err)
	}
	if !tea.BoolValue(saeResp.Body.Success) {
		err = fmt.Errorf("Failed to tag resources, statusCode: %d, code: %s, errCode: %s, message: %s", tea.Int32Value(saeResp.StatusCode), tea.ToString(saeResp.Body.Code), tea.ToString(saeResp.Body.ErrorCode), tea.ToString(saeResp.Body.Message))
		log.Error(err)
		return e.ErrCreateEnv.AddErr(err)
	}

	if isCreateEnv {
		err = commonrepo.NewSAEEnvColl().Create(env)
		if err != nil {
			log.Errorf("[User:%s][EnvName:%s][Product:%s] create sae env error: %s", username, err)
			return e.ErrCreateEnv
		}
	}

	return nil
}

func DeleteSAEEnv(username string, projectName, envName string, log *zap.SugaredLogger) error {
	opt := &commonrepo.SAEEnvFindOptions{ProjectName: projectName, EnvName: envName}
	env, err := commonrepo.NewSAEEnvColl().Find(opt)
	if err != nil {
		err = fmt.Errorf("[User:%s][EnvName:%s][Product:%s] Find SAE Env error: %s", username, envName, projectName, err)
		log.Error(err)
		return e.ErrDeleteEnv.AddErr(err)
	}

	saeModel, err := commonrepo.NewSAEColl().FindDefault()
	if err != nil {
		err = fmt.Errorf("Failed to find default sae, err: %s", err)
		log.Error(err)
		return e.ErrDeleteEnv.AddErr(err)
	}

	saeClient, err := sae.NewClient(saeModel, env.RegionID)
	if err != nil {
		err = fmt.Errorf("Failed to create sae client, err: %s", err)
		log.Error(err)
		return e.ErrDeleteEnv.AddErr(err)
	}

	currentPage := int32(1)
	pageSize := int32(1000)
	for {
		resp, err := ListSAEApps(env.RegionID, env.NamespaceID, projectName, env.EnvName, "", false, currentPage, pageSize, log)
		if err != nil {
			err = fmt.Errorf("Failed to list sae apps, err: %s", err)
			log.Error(err)
			return e.ErrDeleteEnv.AddErr(err)
		}

		resourceIds := "["
		for _, app := range resp.Applications {
			resourceIds += fmt.Sprintf(`"%s",`, app.AppID)
		}
		resourceIds = strings.TrimSuffix(resourceIds, ",") + "]"
		saeRequest := &sae20190506.UntagResourcesRequest{
			RegionId:     tea.String(env.RegionID),
			ResourceType: tea.String("application"),
			TagKeys:      tea.String(fmt.Sprintf(`["%s","%s"]`, setting.SAEZadigProjectTagKey, setting.SAEZadigEnvTagKey)),
			ResourceIds:  tea.String(resourceIds),
		}
		saeResp, err := saeClient.UntagResources(saeRequest)
		if err != nil {
			err = fmt.Errorf("Failed to un tag resources, err: %s", err)
			log.Error(err)
			return e.ErrDeleteEnv.AddErr(err)
		}
		if !tea.BoolValue(saeResp.Body.Success) {
			err = fmt.Errorf("Failed to un tag resources, statusCode: %d, code: %s, errCode: %s, message: %s", tea.Int32Value(saeResp.StatusCode), tea.ToString(saeResp.Body.Code), tea.ToString(saeResp.Body.ErrorCode), tea.ToString(saeResp.Body.Message))
			log.Error(err)
			return e.ErrDeleteEnv.AddErr(err)
		}

		if currentPage*pageSize >= resp.TotalSize {
			break
		}
		currentPage++
		pageSize += 1000
	}

	err = commonrepo.NewSAEEnvColl().Delete(projectName, envName)
	if err != nil {
		log.Errorf("[User:%s][EnvName:%s][Product:%s] delete sae env error: %s", username, envName, projectName, err)
		return e.ErrDeleteEnv
	}

	return nil
}

type ListSAEAppsResponse struct {
	Applications []*models.SAEApplication `json:"applications"`
	CurrentPage  int32                    `json:"current_page"`
	TotalSize    int32                    `json:"total_size"`
}

func ListSAEApps(regionID, namespace, projectName, envName, appName string, isCreateEnv bool, currentPage, pageSize int32, log *zap.SugaredLogger) (*ListSAEAppsResponse, error) {
	saeModel, err := commonrepo.NewSAEColl().FindDefault()
	if err != nil {
		err = fmt.Errorf("Failed to find default sae, err: %s", err)
		log.Error(err)
		return nil, e.ErrListSAEApps.AddErr(err)
	}

	saeClient, err := sae.NewClient(saeModel, regionID)
	if err != nil {
		err = fmt.Errorf("Failed to create sae client, err: %s", err)
		log.Error(err)
		return nil, e.ErrListSAEApps.AddErr(err)
	}

	tags := ""
	if !isCreateEnv {
		tags = fmt.Sprintf(`[{"Key":"%s","Value":"%s"}, {"Key":"%s","Value":"%s"}]`, setting.SAEZadigProjectTagKey, projectName, setting.SAEZadigEnvTagKey, envName)
	}
	saeRequest := &sae20190506.ListApplicationsRequest{
		NamespaceId: tea.String(namespace),
		Tags:        tea.String(tags),
		AppName:     tea.String(appName),
		CurrentPage: tea.Int32(currentPage),
		PageSize:    tea.Int32(pageSize),
	}
	saeResp, err := saeClient.ListApplications(saeRequest)
	if err != nil {
		err = fmt.Errorf("Failed to list applications, err: %s", err)
		log.Error(err)
		return nil, e.ErrListSAEApps.AddErr(err)
	}
	if !tea.BoolValue(saeResp.Body.Success) {
		err = fmt.Errorf("Failed to list applications, statusCode: %d, code: %s, errCode: %s, message: %s", tea.Int32Value(saeResp.StatusCode), tea.ToString(saeResp.Body.Code), tea.ToString(saeResp.Body.ErrorCode), tea.ToString(saeResp.Body.Message))
		log.Error(err)
		return nil, e.ErrListSAEApps.AddErr(err)
	}

	apps := make([]*models.SAEApplication, 0)
	for _, saeApp := range saeResp.Body.Data.Applications {
		tags := make([]*models.SAETag, 0)
		for _, saeTag := range saeApp.Tags {
			tag := &models.SAETag{
				Key:   tea.StringValue(saeTag.Key),
				Value: tea.StringValue(saeTag.Value),
			}
			tags = append(tags, tag)
		}
		app := &models.SAEApplication{
			AppName:          tea.StringValue(saeApp.AppName),
			AppID:            tea.StringValue(saeApp.AppId),
			Tags:             tags,
			Instances:        tea.Int32Value(saeApp.Instances),
			RunningInstances: tea.Int32Value(saeApp.RunningInstances),
			Cpu:              tea.Int32Value(saeApp.Cpu),
			Mem:              tea.Int32Value(saeApp.Mem),
		}
		apps = append(apps, app)
	}

	resp := &ListSAEAppsResponse{
		Applications: apps,
		CurrentPage:  tea.Int32Value(saeResp.Body.CurrentPage),
		TotalSize:    tea.Int32Value(saeResp.Body.TotalSize),
	}

	return resp, nil
}

type SAENamespace struct {
	NameSpaceShortId     string `json:"namespace_short_id"`
	NamespaceName        string `json:"namespace_name"`
	NamespaceId          string `json:"namespace_id"`
	NamespaceDescription string `json:"namespace_description"`
}

func ListSAENamespaces(regionID string, log *zap.SugaredLogger) ([]*SAENamespace, error) {
	saeModel, err := commonrepo.NewSAEColl().FindDefault()
	if err != nil {
		err = fmt.Errorf("Failed to find default sae, err: %s", err)
		log.Error(err)
		return nil, e.ErrListSAEApps.AddErr(err)
	}

	saeClient, err := sae.NewClient(saeModel, regionID)
	if err != nil {
		err = fmt.Errorf("Failed to create sae client, err: %s", err)
		log.Error(err)
		return nil, e.ErrListSAEApps.AddErr(err)
	}

	saeRequest := &sae20190506.DescribeNamespacesRequest{
		CurrentPage: tea.Int32(1),
		PageSize:    tea.Int32(1000),
	}
	saeResp, err := saeClient.DescribeNamespaces(saeRequest)
	if err != nil {
		err = fmt.Errorf("Failed to list namespace, err: %s", err)
		log.Error(err)
		return nil, e.ErrListSAEApps.AddErr(err)
	}
	if !tea.BoolValue(saeResp.Body.Success) {
		err = fmt.Errorf("Failed to list namespace, statusCode: %d, code: %s, errCode: %s, message: %s", tea.Int32Value(saeResp.StatusCode), tea.ToString(saeResp.Body.Code), tea.ToString(saeResp.Body.ErrorCode), tea.ToString(saeResp.Body.Message))
		log.Error(err)
		return nil, e.ErrListSAEApps.AddErr(err)
	}

	resp := make([]*SAENamespace, 0)
	for _, saeNs := range saeResp.Body.Data.Namespaces {
		ns := &SAENamespace{
			NamespaceId:          tea.StringValue(saeNs.NamespaceId),
			NameSpaceShortId:     tea.StringValue(saeNs.NameSpaceShortId),
			NamespaceName:        tea.StringValue(saeNs.NamespaceName),
			NamespaceDescription: tea.StringValue(saeNs.NamespaceDescription),
		}
		resp = append(resp, ns)
	}

	return resp, nil
}

func RestartSAEApp(projectName, envName, appID string, log *zap.SugaredLogger) error {
	opt := &commonrepo.SAEEnvFindOptions{ProjectName: projectName, EnvName: envName}
	env, err := commonrepo.NewSAEEnvColl().Find(opt)
	if err != nil {
		err = fmt.Errorf("Failed to find SAE env, projectName: %s, envName: %s, error: %s", projectName, envName, err)
		log.Error(err)
		return e.ErrRestartService.AddErr(err)
	}

	saeModel, err := commonrepo.NewSAEColl().FindDefault()
	if err != nil {
		err = fmt.Errorf("Failed to find default sae, err: %s", err)
		log.Error(err)
		return e.ErrRestartService.AddErr(err)
	}

	saeClient, err := sae.NewClient(saeModel, env.RegionID)
	if err != nil {
		err = fmt.Errorf("Failed to create sae client, err: %s", err)
		log.Error(err)
		return e.ErrRestartService.AddErr(err)
	}

	saeRequest := &sae20190506.RestartApplicationRequest{
		AppId: tea.String(appID),
	}
	saeResp, err := saeClient.RestartApplication(saeRequest)
	if err != nil {
		err = fmt.Errorf("Failed to restart application %s, err: %s", appID, err)
		log.Error(err)
		return e.ErrRestartService.AddErr(err)
	}
	if !tea.BoolValue(saeResp.Body.Success) {
		err = fmt.Errorf("Failed to restart application %s, statusCode: %d, code: %s, errCode: %s, message: %s", appID, tea.Int32Value(saeResp.StatusCode), tea.ToString(saeResp.Body.Code), tea.ToString(saeResp.Body.ErrorCode), tea.ToString(saeResp.Body.Message))
		log.Error(err)
		return e.ErrRestartService.AddErr(err)
	}

	return nil
}

func RescaleSAEApp(projectName, envName, appID string, Replicas int32, log *zap.SugaredLogger) error {
	opt := &commonrepo.SAEEnvFindOptions{ProjectName: projectName, EnvName: envName}
	env, err := commonrepo.NewSAEEnvColl().Find(opt)
	if err != nil {
		err = fmt.Errorf("Failed to find SAE env, projectName: %s, envName: %s, error: %s", projectName, envName, err)
		log.Error(err)
		return e.ErrScaleService.AddErr(err)
	}

	saeModel, err := commonrepo.NewSAEColl().FindDefault()
	if err != nil {
		err = fmt.Errorf("Failed to find default sae, err: %s", err)
		log.Error(err)
		return e.ErrScaleService.AddErr(err)
	}

	saeClient, err := sae.NewClient(saeModel, env.RegionID)
	if err != nil {
		err = fmt.Errorf("Failed to create sae client, err: %s", err)
		log.Error(err)
		return e.ErrScaleService.AddErr(err)
	}

	saeRequest := &sae20190506.RescaleApplicationRequest{
		AppId:    tea.String(appID),
		Replicas: tea.Int32(Replicas),
	}
	saeResp, err := saeClient.RescaleApplication(saeRequest)
	if err != nil {
		err = fmt.Errorf("Failed to rescale application %s, err: %s", appID, err)
		log.Error(err)
		return e.ErrScaleService.AddErr(err)
	}
	if !tea.BoolValue(saeResp.Body.Success) {
		err = fmt.Errorf("Failed to rescale application %s, statusCode: %d, code: %s, errCode: %s, message: %s", appID, tea.Int32Value(saeResp.StatusCode), tea.ToString(saeResp.Body.Code), tea.ToString(saeResp.Body.ErrorCode), tea.ToString(saeResp.Body.Message))
		log.Error(err)
		return e.ErrScaleService.AddErr(err)
	}

	return nil
}

func RollbackSAEApp(projectName, envName, appID, versionID string, log *zap.SugaredLogger) error {
	opt := &commonrepo.SAEEnvFindOptions{ProjectName: projectName, EnvName: envName}
	env, err := commonrepo.NewSAEEnvColl().Find(opt)
	if err != nil {
		err = fmt.Errorf("Failed to find SAE env, projectName: %s, envName: %s, error: %s", projectName, envName, err)
		log.Error(err)
		return e.ErrRollbackEnvServiceVersion.AddErr(err)
	}

	saeModel, err := commonrepo.NewSAEColl().FindDefault()
	if err != nil {
		err = fmt.Errorf("Failed to find default sae, err: %s", err)
		log.Error(err)
		return e.ErrRollbackEnvServiceVersion.AddErr(err)
	}

	saeClient, err := sae.NewClient(saeModel, env.RegionID)
	if err != nil {
		err = fmt.Errorf("Failed to create sae client, err: %s", err)
		log.Error(err)
		return e.ErrRollbackEnvServiceVersion.AddErr(err)
	}

	saeRequest := &sae20190506.RollbackApplicationRequest{
		AppId:     tea.String(appID),
		VersionId: tea.String(versionID),
	}
	saeResp, err := saeClient.RollbackApplication(saeRequest)
	if err != nil {
		err = fmt.Errorf("Failed to rollback application %s, err: %s", appID, err)
		log.Error(err)
		return e.ErrRollbackEnvServiceVersion.AddErr(err)
	}
	if !tea.BoolValue(saeResp.Body.Success) {
		err = fmt.Errorf("Failed to rollback application %s, statusCode: %d, code: %s, errCode: %s, message: %s", appID, tea.Int32Value(saeResp.StatusCode), tea.ToString(saeResp.Body.Code), tea.ToString(saeResp.Body.ErrorCode), tea.ToString(saeResp.Body.Message))
		log.Error(err)
		return e.ErrRollbackEnvServiceVersion.AddErr(err)
	}

	return nil
}

type SAEAppVersion struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	CreateTime      string `json:"create_time"`
	BuildPackageUrl string `json:"build_package_url"`
	WarUrl          string `json:"war_url"`
}

func ListSAEAppVersions(projectName, envName, appID string, log *zap.SugaredLogger) ([]*SAEAppVersion, error) {
	opt := &commonrepo.SAEEnvFindOptions{ProjectName: projectName, EnvName: envName}
	env, err := commonrepo.NewSAEEnvColl().Find(opt)
	if err != nil {
		err = fmt.Errorf("Failed to find SAE env, projectName: %s, envName: %s, error: %s", projectName, envName, err)
		log.Error(err)
		return nil, e.ErrListEnvServiceVersions.AddErr(err)
	}

	saeModel, err := commonrepo.NewSAEColl().FindDefault()
	if err != nil {
		err = fmt.Errorf("Failed to find default sae, err: %s", err)
		log.Error(err)
		return nil, e.ErrListEnvServiceVersions.AddErr(err)
	}

	saeClient, err := sae.NewClient(saeModel, env.RegionID)
	if err != nil {
		err = fmt.Errorf("Failed to create sae client, err: %s", err)
		log.Error(err)
		return nil, e.ErrRollbackEnvServiceVersion.AddErr(err)
	}

	saeRequest := &sae20190506.ListAppVersionsRequest{
		AppId: tea.String(appID),
	}
	saeResp, err := saeClient.ListAppVersions(saeRequest)
	if err != nil {
		err = fmt.Errorf("Failed to list application %s version, err: %s", appID, err)
		log.Error(err)
		return nil, e.ErrListEnvServiceVersions.AddErr(err)
	}
	if !tea.BoolValue(saeResp.Body.Success) {
		err = fmt.Errorf("Failed to list application %s version, statusCode: %d, code: %s, errCode: %s, message: %s", appID, tea.Int32Value(saeResp.StatusCode), tea.ToString(saeResp.Body.Code), tea.ToString(saeResp.Body.ErrorCode), tea.ToString(saeResp.Body.Message))
		log.Error(err)
		return nil, e.ErrListEnvServiceVersions.AddErr(err)
	}

	resp := make([]*SAEAppVersion, 0)
	for _, saeVersion := range saeResp.Body.Data {
		version := &SAEAppVersion{
			ID:              tea.StringValue(saeVersion.Id),
			Type:            tea.StringValue(saeVersion.Type),
			CreateTime:      tea.StringValue(saeVersion.CreateTime),
			BuildPackageUrl: tea.StringValue(saeVersion.BuildPackageUrl),
			WarUrl:          tea.StringValue(saeVersion.WarUrl),
		}
		resp = append(resp, version)
	}

	return resp, nil
}

type SAEAppInstance struct {
	InstanceId                string `json:"instance_id"`
	GroupID                   string `json:"group_id"`
	InstanceHealthStatus      string `json:"instance_health_status"`
	InstanceContainerStatus   string `json:"instance_container_status"`
	InstanceContainerRestarts int64  `json:"instance_container_restarts"`
	InstanceContainerIp       string `json:"instance_container_ip"`
	Eip                       string `json:"eip"`
	ImageURL                  string `json:"image_url"`
	PackageVersion            string `json:"package_version"`
	CreateTimeStamp           int64  `json:"create_timestamp"`
	FinishTimeStamp           int64  `json:"finish_timestamp"`
}

type SAEAppGroup struct {
	GroupType        int32             `json:"group_type"`
	RunningInstances int32             `json:"running_instances"`
	Replicas         int32             `json:"replicas"`
	GroupId          string            `json:"group_id"`
	GroupName        string            `json:"group_name"`
	PackageType      string            `json:"package_type"`
	PackageVersion   string            `json:"package_version"`
	PackageUrl       string            `json:"package_url"`
	ImageUrl         string            `json:"image_url"`
	Instances        []*SAEAppInstance `json:"instances"`
}

func ListSAEAppInstances(projectName, envName, appID string, log *zap.SugaredLogger) ([]*SAEAppGroup, error) {
	opt := &commonrepo.SAEEnvFindOptions{ProjectName: projectName, EnvName: envName}
	env, err := commonrepo.NewSAEEnvColl().Find(opt)
	if err != nil {
		err = fmt.Errorf("Failed to find SAE env, projectName: %s, envName: %s, error: %s", projectName, envName, err)
		log.Error(err)
		return nil, e.ErrListServicePod.AddErr(err)
	}

	saeModel, err := commonrepo.NewSAEColl().FindDefault()
	if err != nil {
		err = fmt.Errorf("Failed to find default sae, err: %s", err)
		log.Error(err)
		return nil, e.ErrListServicePod.AddErr(err)
	}

	saeClient, err := sae.NewClient(saeModel, env.RegionID)
	if err != nil {
		err = fmt.Errorf("Failed to create sae client, err: %s", err)
		log.Error(err)
		return nil, e.ErrListServicePod.AddErr(err)
	}

	saeRequest := &sae20190506.DescribeApplicationGroupsRequest{
		AppId: tea.String(appID),
	}
	saeResp, err := saeClient.DescribeApplicationGroups(saeRequest)
	if err != nil {
		err = fmt.Errorf("Failed to list application %s group, err: %s", appID, err)
		log.Error(err)
		return nil, e.ErrListServicePod.AddErr(err)
	}
	if !tea.BoolValue(saeResp.Body.Success) {
		err = fmt.Errorf("Failed to list application %s version, statusCode: %d, code: %s, errCode: %s, message: %s", appID, tea.Int32Value(saeResp.StatusCode), tea.ToString(saeResp.Body.Code), tea.ToString(saeResp.Body.ErrorCode), tea.ToString(saeResp.Body.Message))
		log.Error(err)
		return nil, e.ErrListServicePod.AddErr(err)
	}

	resp := make([]*SAEAppGroup, 0)
	for _, saeGroup := range saeResp.Body.Data {
		group := &SAEAppGroup{
			GroupType:        tea.Int32Value(saeGroup.GroupType),
			RunningInstances: tea.Int32Value(saeGroup.RunningInstances),
			Replicas:         tea.Int32Value(saeGroup.Replicas),
			GroupId:          tea.StringValue(saeGroup.GroupId),
			GroupName:        tea.StringValue(saeGroup.GroupName),
			PackageType:      tea.StringValue(saeGroup.PackageType),
			PackageVersion:   tea.StringValue(saeGroup.PackageVersion),
			PackageUrl:       tea.StringValue(saeGroup.PackageUrl),
			ImageUrl:         tea.StringValue(saeGroup.ImageUrl),
		}

		saeRequest := &sae20190506.DescribeApplicationInstancesRequest{
			AppId:   tea.String(appID),
			GroupId: tea.String(group.GroupId),
		}
		saeResp, err := saeClient.DescribeApplicationInstances(saeRequest)
		if err != nil {
			err = fmt.Errorf("Failed to list application %s instance, err: %s", appID, err)
			log.Error(err)
			return nil, e.ErrListServicePod.AddErr(err)
		}
		if !tea.BoolValue(saeResp.Body.Success) {
			err = fmt.Errorf("Failed to list application %s instance, statusCode: %d, code: %s, errCode: %s, message: %s", appID, tea.Int32Value(saeResp.StatusCode), tea.ToString(saeResp.Body.Code), tea.ToString(saeResp.Body.ErrorCode), tea.ToString(saeResp.Body.Message))
			log.Error(err)
			return nil, e.ErrListServicePod.AddErr(err)
		}

		instances := make([]*SAEAppInstance, 0)
		for _, saeInstance := range saeResp.Body.Data.Instances {
			instance := &SAEAppInstance{
				InstanceId:                tea.StringValue(saeInstance.InstanceId),
				GroupID:                   tea.StringValue(saeInstance.GroupId),
				InstanceHealthStatus:      tea.StringValue(saeInstance.InstanceHealthStatus),
				InstanceContainerStatus:   tea.StringValue(saeInstance.InstanceContainerStatus),
				InstanceContainerRestarts: tea.Int64Value(saeInstance.InstanceContainerRestarts),
				InstanceContainerIp:       tea.StringValue(saeInstance.InstanceContainerIp),
				Eip:                       tea.StringValue(saeInstance.Eip),
				ImageURL:                  tea.StringValue(saeInstance.ImageUrl),
				PackageVersion:            tea.StringValue(saeInstance.PackageVersion),
				CreateTimeStamp:           tea.Int64Value(saeInstance.CreateTimeStamp),
				FinishTimeStamp:           tea.Int64Value(saeInstance.FinishTimeStamp),
			}
			instances = append(instances, instance)
		}
		group.Instances = instances

		resp = append(resp, group)
	}

	return resp, nil
}

func RestartSAEAppInstance(projectName, envName, appID, instanceID string, log *zap.SugaredLogger) error {
	opt := &commonrepo.SAEEnvFindOptions{ProjectName: projectName, EnvName: envName}
	env, err := commonrepo.NewSAEEnvColl().Find(opt)
	if err != nil {
		err = fmt.Errorf("Failed to find SAE env, projectName: %s, envName: %s, error: %s", projectName, envName, err)
		log.Error(err)
		return e.ErrRestartService.AddErr(err)
	}

	saeModel, err := commonrepo.NewSAEColl().FindDefault()
	if err != nil {
		err = fmt.Errorf("Failed to find default sae, err: %s", err)
		log.Error(err)
		return e.ErrRestartService.AddErr(err)
	}

	saeClient, err := sae.NewClient(saeModel, env.RegionID)
	if err != nil {
		err = fmt.Errorf("Failed to create sae client, err: %s", err)
		log.Error(err)
		return e.ErrRestartService.AddErr(err)
	}

	saeRequest := &sae20190506.RestartInstancesRequest{
		AppId:       tea.String(appID),
		InstanceIds: tea.String(instanceID),
	}
	saeResp, err := saeClient.RestartInstances(saeRequest)
	if err != nil {
		err = fmt.Errorf("Failed to restart instance, appID: %s, instanceID: %s, err: %s", appID, instanceID, err)
		log.Error(err)
		return e.ErrRestartService.AddErr(err)
	}
	if !tea.BoolValue(saeResp.Body.Success) {
		err = fmt.Errorf("Failed to restart instance, appID: %s, instanceID: %s, statusCode: %d, code: %s, errCode: %s, message: %s", appID, instanceID, tea.Int32Value(saeResp.StatusCode), tea.ToString(saeResp.Body.Code), tea.ToString(saeResp.Body.ErrorCode), tea.ToString(saeResp.Body.Message))
		log.Error(err)
		return e.ErrRestartService.AddErr(err)
	}

	return nil
}

func GetSAEAppInstanceLog(projectName, envName, appID, instanceID string, log *zap.SugaredLogger) (string, error) {
	opt := &commonrepo.SAEEnvFindOptions{ProjectName: projectName, EnvName: envName}
	env, err := commonrepo.NewSAEEnvColl().Find(opt)
	if err != nil {
		err = fmt.Errorf("Failed to find SAE env, projectName: %s, envName: %s, error: %s", projectName, envName, err)
		log.Error(err)
		return "", e.ErrQueryContainerLogs.AddErr(err)
	}

	saeModel, err := commonrepo.NewSAEColl().FindDefault()
	if err != nil {
		err = fmt.Errorf("Failed to find default sae, err: %s", err)
		log.Error(err)
		return "", e.ErrQueryContainerLogs.AddErr(err)
	}

	saeClient, err := sae.NewClient(saeModel, env.RegionID)
	if err != nil {
		err = fmt.Errorf("Failed to create sae client, err: %s", err)
		log.Error(err)
		return "", e.ErrQueryContainerLogs.AddErr(err)
	}

	saeRequest := &sae20190506.DescribeInstanceLogRequest{
		InstanceId: tea.String(instanceID),
	}
	saeResp, err := saeClient.DescribeInstanceLog(saeRequest)
	if err != nil {
		err = fmt.Errorf("Failed to get instance log, appID: %s, instanceID: %s, err: %s", appID, instanceID, err)
		log.Error(err)
		return "", e.ErrQueryContainerLogs.AddErr(err)
	}
	if !tea.BoolValue(saeResp.Body.Success) {
		err = fmt.Errorf("Failed to get instance log, appID: %s, instanceID: %s, statusCode: %d, code: %s, errCode: %s, message: %s", appID, instanceID, tea.Int32Value(saeResp.StatusCode), tea.ToString(saeResp.Body.Code), tea.ToString(saeResp.Body.ErrorCode), tea.ToString(saeResp.Body.Message))
		log.Error(err)
		return "", e.ErrQueryContainerLogs.AddErr(err)
	}

	return tea.StringValue(saeResp.Body.Data), nil
}
