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
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/sets"

	commonmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	commonrepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	commonservice "github.com/koderover/zadig/pkg/microservice/aslan/core/common/service"
	commonutil "github.com/koderover/zadig/pkg/microservice/aslan/core/common/util"
	"github.com/koderover/zadig/pkg/setting"
	"github.com/koderover/zadig/pkg/shared/client/systemconfig"
	e "github.com/koderover/zadig/pkg/tool/errors"
	"github.com/koderover/zadig/pkg/tool/log"
	"github.com/koderover/zadig/pkg/types"
)

type BuildResp struct {
	ID          string                              `json:"id"`
	Name        string                              `json:"name"`
	Targets     []*commonmodels.ServiceModuleTarget `json:"targets"`
	KeyVals     []*commonmodels.KeyVal              `json:"key_vals"`
	UpdateTime  int64                               `json:"update_time"`
	UpdateBy    string                              `json:"update_by"`
	Pipelines   []string                            `json:"pipelines"`
	ProductName string                              `json:"productName"`
}

type ServiceModuleAndBuildResp struct {
	ServiceName   string       `json:"service_name"`
	ServiceModule string       `json:"service_module"`
	ModuleBuilds  []*BuildResp `json:"module_builds"`
}

func FindBuild(name, productName string, log *zap.SugaredLogger) (*commonmodels.Build, error) {
	opt := &commonrepo.BuildFindOption{
		Name:        name,
		ProductName: productName,
	}

	resp, err := commonrepo.NewBuildColl().Find(opt)
	if err != nil {
		log.Errorf("[Build.Find] %s error: %v", name, err)
		return nil, e.ErrGetBuildModule.AddErr(err)
	}

	commonservice.EnsureResp(resp)

	return resp, nil
}

func ListBuild(name, targets, productName string, log *zap.SugaredLogger) ([]*BuildResp, error) {
	opt := &commonrepo.BuildListOption{
		Name:        name,
		ProductName: productName,
	}

	if len(strings.TrimSpace(targets)) != 0 {
		opt.Targets = strings.Split(targets, ",")
	}

	currentProductBuilds, err := commonrepo.NewBuildColl().List(opt)
	if err != nil {
		log.Errorf("[Pipeline.List] %s error: %v", name, err)
		return nil, e.ErrListBuildModule.AddErr(err)
	}
	// 获取全部 pipeline
	pipes, err := commonrepo.NewPipelineColl().List(&commonrepo.PipelineListOption{IsPreview: true})
	if err != nil {
		log.Errorf("[Pipeline.List] %s error: %v", name, err)
		return nil, e.ErrListBuildModule.AddErr(err)
	}

	resp := make([]*BuildResp, 0)
	for _, build := range currentProductBuilds {
		b := &BuildResp{
			ID:          build.ID.Hex(),
			Name:        build.Name,
			Targets:     build.Targets,
			UpdateTime:  build.UpdateTime,
			UpdateBy:    build.UpdateBy,
			ProductName: build.ProductName,
			Pipelines:   []string{},
		}

		for _, pipe := range pipes {
			// current build module used by this pipeline
			for _, serviceModuleTarget := range b.Targets {
				if serviceModuleTarget.ServiceModule == pipe.Target {
					b.Pipelines = append(b.Pipelines, pipe.Name)
				}
			}
		}
		resp = append(resp, b)
	}

	return resp, nil
}

func ListBuildModulesByServiceModule(productName, filterJenkins string, log *zap.SugaredLogger) ([]*ServiceModuleAndBuildResp, error) {
	services, err := commonrepo.NewServiceColl().ListMaxRevisionsByProduct(productName)
	if err != nil {
		return nil, e.ErrListBuildModule.AddErr(err)
	}

	serviceModuleAndBuildResp := make([]*ServiceModuleAndBuildResp, 0)
	for _, serviceTmpl := range services {
		for _, container := range serviceTmpl.Containers {
			opt := &commonrepo.BuildListOption{
				ServiceName: serviceTmpl.ServiceName,
				Targets:     []string{container.Name},
			}
			if serviceTmpl.Visibility != setting.PublicService {
				opt.ProductName = productName
			}

			buildModules, err := commonrepo.NewBuildColl().List(opt)
			if err != nil {
				return nil, e.ErrListBuildModule.AddErr(err)
			}
			var resp []*BuildResp
			for _, build := range buildModules {
				if filterJenkins == "true" && build.JenkinsBuild != nil {
					continue
				}
				// get build env vars when it's a template build
				if build.TemplateID != "" {
					for _, target := range build.Targets {
						if target.ServiceModule == container.Name && target.ServiceName == serviceTmpl.ServiceName {
							build.PreBuild.Envs = target.Envs
						}
					}
				}
				resp = append(resp, &BuildResp{
					ID:      build.ID.Hex(),
					Name:    build.Name,
					KeyVals: build.PreBuild.Envs,
				})
			}
			serviceModuleAndBuildResp = append(serviceModuleAndBuildResp, &ServiceModuleAndBuildResp{
				ServiceName:   serviceTmpl.ServiceName,
				ServiceModule: container.Name,
				ModuleBuilds:  resp,
			})
		}
	}
	return serviceModuleAndBuildResp, nil
}

func fillBuildTargetData(build *commonmodels.Build) error {
	if build.TemplateID == "" {
		return nil
	}
	buildTemplate, err := commonrepo.NewBuildTemplateColl().Find(&commonrepo.BuildTemplateQueryOption{
		ID: build.TemplateID,
	})
	if err != nil {
		return fmt.Errorf("failed to find build template with id: %s, err: %s", build.TemplateID, err)
	}
	build.Targets = make([]*commonmodels.ServiceModuleTarget, 0, len(build.TargetRepos))
	for _, target := range build.TargetRepos {
		build.Targets = append(build.Targets, &commonmodels.ServiceModuleTarget{
			ProductName:   target.Service.ProductName,
			ServiceName:   target.Service.ServiceName,
			ServiceModule: target.Service.ServiceModule,
			Repos:         target.Repos,
			Envs:          commonservice.MergeBuildEnvs(buildTemplate.PreBuild.Envs, target.Envs),
		})
	}
	return nil
}

func CreateBuild(username string, build *commonmodels.Build, log *zap.SugaredLogger) error {
	if len(build.Name) == 0 {
		return e.ErrCreateBuildModule.AddDesc("empty name")
	}
	if err := commonutil.CheckDefineResourceParam(build.PreBuild.ResReq, build.PreBuild.ResReqSpec); err != nil {
		return e.ErrCreateBuildModule.AddDesc(err.Error())
	}

	build.UpdateBy = username
	err := correctFields(build)
	if err != nil {
		return err
	}

	if err := commonrepo.NewBuildColl().Create(build); err != nil {
		log.Errorf("[Build.Upsert] %s error: %v", build.Name, err)
		return e.ErrCreateBuildModule.AddErr(err)
	}

	return nil
}

func UpdateBuild(username string, build *commonmodels.Build, log *zap.SugaredLogger) error {
	if len(build.Name) == 0 {
		return e.ErrUpdateBuildModule.AddDesc("empty name")
	}
	if err := commonutil.CheckDefineResourceParam(build.PreBuild.ResReq, build.PreBuild.ResReqSpec); err != nil {
		return e.ErrUpdateBuildModule.AddDesc(err.Error())
	}

	existed, err := commonrepo.NewBuildColl().Find(&commonrepo.BuildFindOption{Name: build.Name, ProductName: build.ProductName})
	if err == nil && existed.PreBuild != nil && build.PreBuild != nil {
		commonservice.EnsureSecretEnvs(existed.PreBuild.Envs, build.PreBuild.Envs)
	}

	err = correctFields(build)
	if err != nil {
		return err
	}

	if err = updateCvmService(build, existed); err != nil {
		log.Warnf("failed to update cvm service,err:%s", err)
	}

	build.UpdateBy = username
	build.UpdateTime = time.Now().Unix()
	if err := commonrepo.NewBuildColl().Update(build); err != nil {
		log.Errorf("[Build.Upsert] %s error: %v", build.Name, err)
		return e.ErrUpdateBuildModule.AddErr(err)
	}

	return nil
}

func updateCvmService(currentBuild, oldBuild *commonmodels.Build) error {
	deleteServices := sets.NewString()
	currentServiceModuleKey := sets.NewString()
	for _, currentServiceModule := range currentBuild.Targets {
		currentServiceModuleKey.Insert(fmt.Sprintf("%s-%s-%s", currentServiceModule.ProductName, currentServiceModule.ServiceName, currentServiceModule.ServiceModule))
	}

	for _, oldServiceModule := range oldBuild.Targets {
		if !currentServiceModuleKey.Has(fmt.Sprintf("%s-%s-%s", oldServiceModule.ProductName, oldServiceModule.ServiceName, oldServiceModule.ServiceModule)) {
			deleteServices.Insert(oldServiceModule.ServiceName)
		}
	}

	for _, serviceName := range deleteServices.List() {
		opt := &commonrepo.ServiceFindOption{
			ServiceName:   serviceName,
			Type:          setting.PMDeployType,
			ProductName:   currentBuild.ProductName,
			ExcludeStatus: setting.ProductStatusDeleting,
		}

		resp, err := commonrepo.NewServiceColl().Find(opt)
		if err != nil {
			continue
		}

		serviceTemplate := fmt.Sprintf(setting.ServiceTemplateCounterName, resp.ServiceName, resp.ProductName)
		rev, err := commonrepo.NewCounterColl().GetNextSeq(serviceTemplate)
		if err != nil {
			return err
		}
		resp.Revision = rev

		if err := commonrepo.NewServiceColl().Delete(resp.ServiceName, resp.Type, resp.ProductName, setting.ProductStatusDeleting, resp.Revision); err != nil {
			log.Errorf("failed to delete service %s, error: %s", resp.ServiceName, err)
			return err
		}
		resp.BuildName = ""
		if err := commonrepo.NewServiceColl().Create(resp); err != nil {
			log.Errorf("failed to delete service %s, error: %s", resp.ServiceName, err)
			return err
		}
	}

	return nil
}

func DeleteBuild(name, productName string, log *zap.SugaredLogger) error {
	if len(name) == 0 {
		return e.ErrDeleteBuildModule.AddDesc("empty name")
	}

	existed, err := FindBuild(name, productName, log)
	if err != nil {
		log.Errorf("[Build.Delete] %s error: %v", name, err)
		return e.ErrDeleteBuildModule.AddErr(err)
	}

	// 如果使用过编译模块
	if len(existed.Targets) != 0 {
		targets := sets.String{}
		for _, target := range existed.Targets {
			if !targets.Has(target.ServiceModule) {
				targets.Insert(target.ServiceModule)
			}
		}
		opt := &commonrepo.PipelineListOption{
			Targets: targets.List(),
		}

		// 获取全部 pipeline
		pipes, err := commonrepo.NewPipelineColl().List(opt)
		if err != nil {
			log.Errorf("[Pipeline.List] %s error: %v", name, err)
			return e.ErrDeleteBuildModule.AddErr(err)
		}

		if len(pipes) > 0 {
			var pipeNames []string
			for _, pipe := range pipes {
				pipeNames = append(pipeNames, pipe.Name)
			}
			msg := fmt.Sprintf("build module used by pipelines %v", pipeNames)
			return e.ErrDeleteBuildModule.AddDesc(msg)
		}
	}
	services, _ := commonrepo.NewServiceColl().ListMaxRevisions(&commonrepo.ServiceListOption{BuildName: name, ProductName: productName})
	serviceNames := make([]string, 0)
	for _, service := range services {
		serviceNames = append(serviceNames, service.ServiceName)
	}
	if len(serviceNames) > 0 {
		return e.ErrDeleteBuildModule.AddDesc(fmt.Sprintf("该构建被服务 [%s] 引用，请解除引用之后再做删除!", strings.Join(serviceNames, ",")))
	}
	// 删除服务配置，检查工作流是否有引用该编译模板，需要二次确认
	if err := commonrepo.NewBuildColl().Delete(name, productName); err != nil {
		log.Errorf("[Build.Delete] %s error: %v", name, err)
		return e.ErrDeleteBuildModule.AddErr(err)
	}
	return nil
}

func handleServiceTargets(name, productName string, targets []*commonmodels.ServiceModuleTarget) {
	var preTargets []*commonmodels.ServiceModuleTarget
	if preBuild, err := commonrepo.NewBuildColl().Find(&commonrepo.BuildFindOption{Name: name, ProductName: productName}); err == nil {
		preTargets = preBuild.Targets
	}

	preServiceModuleTargetMap := make(map[string]*commonmodels.ServiceModuleTarget)
	for _, preServiceModuleTarget := range preTargets {
		target := fmt.Sprintf("%s-%s-%s", preServiceModuleTarget.ProductName, preServiceModuleTarget.ServiceName, preServiceModuleTarget.ServiceModule)
		preServiceModuleTargetMap[target] = preServiceModuleTarget
	}

	modifyServiceModuleTargetMap := make(map[string]*commonmodels.ServiceModuleTarget)
	for _, modifyServiceModuleTarget := range targets {
		target := fmt.Sprintf("%s-%s-%s", modifyServiceModuleTarget.ProductName, modifyServiceModuleTarget.ServiceName, modifyServiceModuleTarget.ServiceModule)
		modifyServiceModuleTargetMap[target] = modifyServiceModuleTarget
	}

	deleteTargets := make([]*commonmodels.ServiceModuleTarget, 0)
	for _, deleteTarget := range preTargets {
		target := fmt.Sprintf("%s-%s-%s", deleteTarget.ProductName, deleteTarget.ServiceName, deleteTarget.ServiceModule)
		if _, isExist := modifyServiceModuleTargetMap[target]; !isExist {
			deleteTargets = append(deleteTargets, deleteTarget)
		}
	}

	addTargets := make([]*commonmodels.ServiceModuleTarget, 0)
	for _, addTarget := range targets {
		target := fmt.Sprintf("%s-%s-%s", addTarget.ProductName, addTarget.ServiceName, addTarget.ServiceModule)
		if _, isExist := preServiceModuleTargetMap[target]; !isExist {
			addTargets = append(addTargets, addTarget)
		}
	}

	services := make([]*commonmodels.Service, 0)
	for _, target := range deleteTargets {
		service, err := commonrepo.NewServiceColl().Find(
			&commonrepo.ServiceFindOption{
				ServiceName:   target.ServiceName,
				ProductName:   productName,
				ExcludeStatus: setting.ProductStatusDeleting,
				Type:          setting.PMDeployType,
			})
		if err == nil {
			services = append(services, service)
		}
	}

	addServices := make([]*commonmodels.Service, 0)
	for _, target := range addTargets {
		service, err := commonrepo.NewServiceColl().Find(
			&commonrepo.ServiceFindOption{
				ServiceName:   target.ServiceName,
				ProductName:   productName,
				ExcludeStatus: setting.ProductStatusDeleting,
				Type:          setting.PMDeployType,
			})
		if err == nil {
			addServices = append(addServices, service)
		}
	}

	for _, args := range services {
		serviceTemplate := fmt.Sprintf(setting.ServiceTemplateCounterName, args.ServiceName, args.ProductName)
		rev, err := commonrepo.NewCounterColl().GetNextSeq(serviceTemplate)
		if err != nil {
			continue
		}
		args.Revision = rev
		args.BuildName = ""

		if err := commonrepo.NewServiceColl().Delete(args.ServiceName, args.Type, args.ProductName, setting.ProductStatusDeleting, args.Revision); err != nil {
			continue
		}

		if err := commonrepo.NewServiceColl().Create(args); err != nil {
			continue
		}
	}

	for _, args := range addServices {
		serviceTemplate := fmt.Sprintf(setting.ServiceTemplateCounterName, args.ServiceName, args.ProductName)
		rev, err := commonrepo.NewCounterColl().GetNextSeq(serviceTemplate)
		if err != nil {
			continue
		}
		args.Revision = rev
		args.BuildName = name

		if err := commonrepo.NewServiceColl().Create(args); err != nil {
			continue
		}
	}
}

func UpdateBuildTargets(name, productName string, targets []*commonmodels.ServiceModuleTarget, log *zap.SugaredLogger) error {
	if err := verifyBuildTargets(name, productName, targets, log); err != nil {
		return e.ErrUpdateBuildParam.AddErr(err)
	}

	//处理云主机服务组件逻辑
	handleServiceTargets(name, productName, targets)

	err := commonrepo.NewBuildColl().UpdateTargets(name, productName, targets)
	if err != nil {
		log.Errorf("[Build.UpdateServices] %s error: %v", name, err)
		return e.ErrUpdateBuildServiceTmpls.AddErr(err)
	}
	return nil
}

func correctFields(build *commonmodels.Build) error {
	err := fillBuildTargetData(build)
	if err != nil {
		return err
	}
	// make sure cache has no empty field
	caches := make([]string, 0)
	for _, cache := range build.Caches {
		cache = strings.Trim(cache, " /")
		if cache != "" {
			caches = append(caches, cache)
		}
	}
	build.Caches = caches

	// trim the docker file and context
	if build.PostBuild != nil && build.PostBuild.DockerBuild != nil {
		build.PostBuild.DockerBuild.DockerFile = strings.Trim(build.PostBuild.DockerBuild.DockerFile, " ")
		build.PostBuild.DockerBuild.WorkDir = strings.Trim(build.PostBuild.DockerBuild.WorkDir, " ")
	}
	if build.TemplateID == "" {
		for _, repo := range build.Repos {
			if repo.Source != setting.SourceFromOther {
				continue
			}
			modifyAuthType(repo)
		}
		return nil
	}

	for _, target := range build.Targets {
		for _, repo := range target.Repos {
			if repo.Source != setting.SourceFromOther {
				continue
			}
			modifyAuthType(repo)
		}
	}
	return nil
}

func modifyAuthType(repo *types.Repository) {
	repo.RepoOwner = strings.TrimPrefix(repo.RepoOwner, "/")
	repo.RepoOwner = strings.TrimSuffix(repo.RepoOwner, "/")
	repo.RepoName = strings.TrimPrefix(repo.RepoName, "/")
	repo.RepoName = strings.TrimSuffix(repo.RepoName, "/")
	codehosts, err := systemconfig.New().ListCodeHostsInternal()
	if err != nil {
		log.Errorf("failed to list codehost,err:%s", err)
	}
	for _, codehost := range codehosts {
		if repo.CodehostID == codehost.ID {
			repo.AuthType = codehost.AuthType
			break
		}
	}
}

func verifyBuildTargets(name, productName string, targets []*commonmodels.ServiceModuleTarget, log *zap.SugaredLogger) error {
	if hasDuplicateTargets(targets) {
		return errors.New("duplicate target found")
	}

	existed, err := commonrepo.NewBuildColl().DistinctTargets([]string{name}, productName)
	if err != nil {
		log.Errorf("[Build.DistinctTargets] error: %v", err)
		return err
	}

	for _, serviceModuleTarget := range targets {
		target := fmt.Sprintf("%s-%s-%s", serviceModuleTarget.ProductName, serviceModuleTarget.ServiceName, serviceModuleTarget.ServiceModule)
		if _, ok := existed[target]; ok {
			return fmt.Errorf("target already existed: %s", target)
		}
	}
	return nil
}

func hasDuplicateTargets(serviceModuleTargets []*commonmodels.ServiceModuleTarget) bool {
	tMap := make(map[string]bool)
	for _, serviceModuleTarget := range serviceModuleTargets {
		target := fmt.Sprintf("%s-%s-%s", serviceModuleTarget.ProductName, serviceModuleTarget.ServiceName, serviceModuleTarget.ServiceModule)
		if _, ok := tMap[target]; ok {
			return true
		}
		tMap[target] = true
	}
	return false
}
