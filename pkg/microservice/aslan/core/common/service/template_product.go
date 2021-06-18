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
	"sync"

	"go.uber.org/zap"

	"github.com/koderover/zadig/pkg/internal/poetry"
	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/dao/models/template"
	commonrepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/dao/repo"
	templaterepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/dao/repo/template"
	"github.com/koderover/zadig/pkg/microservice/aslan/internal/cache"
	"github.com/koderover/zadig/pkg/setting"
	e "github.com/koderover/zadig/pkg/tool/errors"
)

type PipelineResource struct {
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

type Features struct {
	Features []string `json:"features"`
}

func GetProductTemplate(productName string, log *zap.SugaredLogger) (*template.Product, error) {
	resp, err := templaterepo.NewProductColl().Find(productName)
	if err != nil {
		log.Errorf("GetProductTemplate error: %v", err)
		return nil, e.ErrGetProduct.AddDesc(err.Error())
	}

	err = FillProductTemplateVars([]*template.Product{resp}, log)
	if err != nil {
		return nil, fmt.Errorf("FillProductTemplateVars err : %v", err)
	}

	totalServices, err := commonrepo.NewServiceColl().DistinctServices(&commonrepo.ServiceListOption{ProductName: productName, IsSort: true, ExcludeStatus: setting.ProductStatusDeleting})
	if err != nil {
		return resp, fmt.Errorf("DistinctServices err : %v", err)
	}

	totalBuilds, err := commonrepo.NewBuildColl().List(&commonrepo.BuildListOption{ProductName: productName, IsSort: true})
	if err != nil {
		return resp, fmt.Errorf("Build.List err : %v", err)
	}

	totalTests, err := commonrepo.NewTestingColl().List(&commonrepo.ListTestOption{ProductName: productName, IsSort: true})
	if err != nil {
		return resp, fmt.Errorf("Testing.List err : %v", err)
	}

	totalEnvs, err := commonrepo.NewProductColl().List(&commonrepo.ProductListOptions{Name: productName, IsSort: true})
	if err != nil {
		return resp, fmt.Errorf("Product.List err : %v", err)
	}

	totalWorkflows, err := commonrepo.NewWorkflowColl().List(&commonrepo.ListWorkflowOption{ProductName: productName, IsSort: true})
	if err != nil {
		return resp, fmt.Errorf("Workflow.List err : %v", err)
	}

	totalPiplines, err := commonrepo.NewPipelineColl().List(&commonrepo.PipelineListOption{ProductName: productName, IsPreview: true, IsDeleted: false})
	if err != nil {
		return resp, fmt.Errorf("Pipeline.List err : %v", err)
	}

	totalEnvTemplateServiceNum := 0
	for _, services := range resp.Services {
		totalEnvTemplateServiceNum += len(services)
	}

	resp.TotalServiceNum = len(totalServices)
	if len(totalServices) > 0 {
		serviceObj, err := GetServiceTemplate(totalServices[0].ServiceName, totalServices[0].Type, productName, setting.ProductStatusDeleting, totalServices[0].Revision, log)
		if err != nil {
			return resp, fmt.Errorf("GetServiceTemplate err : %v", err)
		}
		resp.LatestServiceUpdateTime = serviceObj.CreateTime
		resp.LatestServiceUpdateBy = serviceObj.CreateBy
	}
	resp.TotalBuildNum = len(totalBuilds)
	if len(totalBuilds) > 0 {
		resp.LatestBuildUpdateTime = totalBuilds[0].UpdateTime
		resp.LatestBuildUpdateBy = totalBuilds[0].UpdateBy
	}
	resp.TotalTestNum = len(totalTests)
	if len(totalTests) > 0 {
		resp.LatestTestUpdateTime = totalTests[0].UpdateTime
		resp.LatestTestUpdateBy = totalTests[0].UpdateBy
	}
	resp.TotalEnvNum = len(totalEnvs)
	if len(totalEnvs) > 0 {
		resp.LatestEnvUpdateTime = totalEnvs[0].UpdateTime
		resp.LatestEnvUpdateBy = totalEnvs[0].UpdateBy
	}
	resp.TotalWorkflowNum = len(totalWorkflows) + len(totalPiplines)
	if len(totalWorkflows) > 0 {
		resp.LatestWorkflowUpdateTime = totalWorkflows[0].UpdateTime
		resp.LatestWorkflowUpdateBy = totalWorkflows[0].UpdateBy
	}
	resp.TotalEnvTemplateServiceNum = totalEnvTemplateServiceNum

	return resp, nil
}

func GetFeatures(log *zap.SugaredLogger) (string, error) {
	featuresByteKey := []byte("features")
	featuresByteValue, err := cache.Get(featuresByteKey)
	if err != nil {
		poetryCtl := poetry.New(config.PoetryAPIServer(), config.PoetryAPIRootKey())
		fs, err := poetryCtl.ListFeatures()
		if err != nil {
			return "", err
		}
		cacheValue := strings.Join(fs, ",")
		// 一天过期
		if err = cache.Set(featuresByteKey, []byte(cacheValue), 86400); err != nil {
			log.Errorf("getFeatures set cache err:%v", err)
		}
		return cacheValue, nil
	}

	return string(featuresByteValue), nil
}

func FillProductTemplateVars(productTemplates []*template.Product, log *zap.SugaredLogger) error {
	var (
		wg            sync.WaitGroup
		maxRoutineNum = 20                            // 协程池最大协程数量
		ch            = make(chan int, maxRoutineNum) // 控制协程数量
		errStr        string
	)

	defer close(ch)

	for _, tmpl := range productTemplates {
		wg.Add(1)
		ch <- 1

		go func(tmpl *template.Product) {
			defer func() {
				<-ch
				wg.Done()
			}()
			renderSet, err := GetRenderSet(tmpl.ProductName, 0, log)
			if err != nil {
				errStr += fmt.Sprintf("Failed to find render set for product template, productName:%s, err:%v\n", tmpl.ProductName, err)
				log.Errorf("Failed to find render set for product template %s", tmpl.ProductName)
				return
			}
			tmpl.Vars = make([]*template.RenderKV, 0)
			for _, kv := range renderSet.KVs {
				tmpl.Vars = append(tmpl.Vars, &template.RenderKV{
					Key:      kv.Key,
					Value:    kv.Value,
					State:    string(kv.State),
					Alias:    kv.Alias,
					Services: kv.Services,
				})
			}
		}(tmpl)
	}

	wg.Wait()
	if errStr != "" {
		return e.ErrGetRenderSet.AddDesc(errStr)
	}

	return nil
}
