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
	"context"
	"fmt"

	"go.uber.org/zap"

	commonmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	commonrepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	templaterepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb/template"
	commonutil "github.com/koderover/zadig/pkg/microservice/aslan/core/common/util"
	"github.com/koderover/zadig/pkg/setting"
	e "github.com/koderover/zadig/pkg/tool/errors"
	"github.com/koderover/zadig/pkg/util"
)

func ListProductsRevision(productName, envName string, log *zap.SugaredLogger) (prodRevs []*ProductRevision, err error) {
	products, err := commonrepo.NewProductColl().List(&commonrepo.ProductListOptions{Name: productName, IsSortByProductName: true, EnvName: envName, ExcludeStatus: []string{setting.ProductStatusDeleting, setting.ProductStatusUnknown}})
	if err != nil {
		return nil, e.ErrListProducts.AddDesc(err.Error())
	}

	// list services with max revision of project
	allServiceTmpls, err := getServicesWithMaxRevision(productName)
	if err != nil {
		log.Errorf("ListAllRevisions error: %v", err)
		return prodRevs, e.ErrListProducts.AddDesc(err.Error())
	}

	for _, prod := range products {
		if prod.Production {
			continue
		}
		prodRev, err := GetProductRevision(prod, allServiceTmpls, log)
		if err != nil {
			log.Error(err)
			return prodRevs, err
		}
		prodRevs = append(prodRevs, prodRev)
	}
	return prodRevs, nil
}

// ListProductSnapsByOption called by service cron
func ListProductSnapsByOption(deployType string, log *zap.SugaredLogger) ([]*ProductRevision, error) {
	var (
		err          error
		prodRevs     = make([]*ProductRevision, 0)
		products     = make([]*commonmodels.Product, 0)
		projectNames = make([]string, 0)
	)

	listOption := &templaterepo.ProductListOpt{}
	switch deployType {
	case setting.PMDeployType:
		listOption = &templaterepo.ProductListOpt{BasicFacility: setting.BasicFacilityCVM}
	default:
		listOption = &templaterepo.ProductListOpt{DeployType: deployType}
	}

	temProducts, err := templaterepo.NewProductColl().ListWithOption(listOption)
	if err != nil {
		log.Errorf("Collection.TemplateProduct.List error: %s", err)
		return prodRevs, e.ErrListProducts.AddDesc(err.Error())
	}

	for _, v := range temProducts {
		projectNames = append(projectNames, v.ProductName)
	}
	if len(projectNames) == 0 {
		return prodRevs, nil
	}

	products, err = commonrepo.NewProductColl().List(&commonrepo.ProductListOptions{
		ExcludeStatus: []string{setting.ProductStatusDeleting, setting.ProductStatusUnknown},
		InProjects:    projectNames,
		Production:    util.GetBoolPointer(false),
	})
	if err != nil {
		log.Errorf("Collection.Product.List error: %s", err)
		return prodRevs, e.ErrListProducts.AddDesc(err.Error())
	}

	for _, prod := range products {
		prodRev := &ProductRevision{
			ProductName: prod.ProductName,
			EnvName:     prod.EnvName,
		}
		for _, svc := range prod.GetServiceMap() {
			prodRev.ServiceRevisions = append(prodRev.ServiceRevisions, &SvcRevision{
				ServiceName:     svc.ServiceName,
				Type:            svc.Type,
				CurrentRevision: svc.Revision,
			})
		}
		prodRevs = append(prodRevs, prodRev)
	}
	return prodRevs, nil
}

func GetProductRevision(product *commonmodels.Product, allServiceTmpls []*commonmodels.Service, log *zap.SugaredLogger) (*ProductRevision, error) {
	prodRev := new(ProductRevision)

	productTemplateName := product.ProductName
	prodTmpl, err := templaterepo.NewProductColl().Find(productTemplateName)
	if err != nil {
		log.Errorf("[ProductTmpl.Find] %s error: %v", product.ProductName, err)
		return prodRev, e.ErrFindProductTmpl
	}

	prodRev.EnvName = product.EnvName
	prodRev.ID = product.ID.Hex()
	prodRev.ProductName = product.ProductName
	prodRev.CurrentRevision = product.Revision
	prodRev.NextRevision = prodTmpl.Revision
	prodRev.ServiceRevisions = make([]*SvcRevision, 0)
	prodRev.IsPublic = product.IsPublic

	product.EnsureRenderInfo()
	if product.Source == setting.SourceFromExternal {
		return prodRev, nil
	}

	if prodRev.NextRevision > prodRev.CurrentRevision {
		prodRev.Updatable = true
	}

	// 交叉对比已创建的服务组和服务组模板
	prodRev.ServiceRevisions, err = compareGroupServicesRev(prodTmpl.Services, product, allServiceTmpls, log)
	if err != nil {
		log.Error(err)
		return nil, e.ErrGetProductRevision.AddDesc(err.Error())
	}

	//set service deploy strategy info
	for _, svc := range prodRev.ServiceRevisions {
		svc.DeployStrategy = commonutil.GetServiceDeployStrategy(svc.ServiceName, product.ServiceDeployStrategy)
	}

	if !prodRev.Updatable {
		prodRev.Updatable = prodRev.GroupsUpdated()
	}

	if !prodRev.Updatable {
		for _, svc := range product.GetServiceMap() {
			if !commonutil.ServiceDeployed(svc.ServiceName, product.ServiceDeployStrategy) {
				prodRev.Updatable = true
				break
			}
		}
	}

	return prodRev, nil
}

// compareGroupServicesRev 服务二维数组对比revision
// parameters:
// - servicesTmpl: service templates 2-dimensional array from product template
// - product: the product environment instance
// - maxServices: distinted service and max revision
// - maxConfigs: distincted service config and max revision
func compareGroupServicesRev(servicesTmpl [][]string, productInfo *commonmodels.Product, allServiceTmpls []*commonmodels.Service, log *zap.SugaredLogger) ([]*SvcRevision, error) {

	var serviceRev []*SvcRevision
	svcList := make([]*commonmodels.ProductService, 0)
	svcTmplNameList := make([]string, 0)

	for _, services := range productInfo.Services {
		svcList = append(svcList, services...)
	}

	for _, svcsTmpl := range servicesTmpl {
		svcTmplNameList = append(svcTmplNameList, svcsTmpl...)
	}

	// Note: For sub env, only the services in the base env are displayed.
	if productInfo.ShareEnv.Enable && !productInfo.ShareEnv.IsBase {
		svcGroupsInBase, err := GetEnvServiceList(context.TODO(), productInfo.ProductName, productInfo.ShareEnv.BaseEnv)
		if err != nil {
			return nil, fmt.Errorf("failed to get service list in base env %q of product %q: %s", productInfo.EnvName, productInfo.ProductName, err)
		}

		svcMap := map[string]struct{}{}
		for _, svcGroup := range svcGroupsInBase {
			for _, svcName := range svcGroup {
				svcMap[svcName] = struct{}{}
			}
		}

		tmplNameList := []string{}
		for _, svcTmplName := range svcTmplNameList {
			if _, found := svcMap[svcTmplName]; found {
				tmplNameList = append(tmplNameList, svcTmplName)
			}
		}

		svcTmplNameList = tmplNameList
	}

	var err error
	serviceRev, err = compareServicesRev(svcTmplNameList, svcList, allServiceTmpls, log)
	if err != nil {
		log.Errorf("Failed to compare service revision, %s:%s, Error: %v", productInfo.ProductName, productInfo.EnvName, err)
		return serviceRev, e.ErrListProductsRevision.AddDesc(err.Error())
	}
	return serviceRev, nil
}

// compareServicesRev 拍平后服务数组对比revision
// parameters:
// - serviceTmplNames: service names from product-service template
// - services: service list of product environment instance
// - maxServices: distinted service and max revision
// - maxConfigs: distincted service config and max revision
func compareServicesRev(serviceTmplNames []string, productServices []*commonmodels.ProductService, allServiceTmpls []*commonmodels.Service, log *zap.SugaredLogger) ([]*SvcRevision, error) {

	serviceRevs := make([]*SvcRevision, 0)

	productServiceMap := make(map[string]*commonmodels.ProductService)
	for _, service := range productServices {
		productServiceMap[service.ServiceName] = service
	}

	serviceTmplMap := make(map[string]string)
	for _, tmplName := range serviceTmplNames {
		serviceTmplMap[tmplName] = tmplName
	}

	latestSvcTmplMap := make(map[string]*commonmodels.Service)
	for _, serviceTmpl := range allServiceTmpls {
		latestSvcTmplMap[serviceTmpl.ServiceName] = serviceTmpl
	}

	// services exist in template_service but not in product_service
	// these services can be added into product
	for _, serviceTmplName := range serviceTmplNames {
		if _, ok := productServiceMap[serviceTmplName]; !ok {
			latestSvcTmpl, ok := latestSvcTmplMap[serviceTmplName]
			if !ok {
				log.Errorf("service %s not found in allServiceTmpls", serviceTmplName)
				return serviceRevs, fmt.Errorf("failed to find service: %s in allServiceTmpls", serviceTmplName)
			}
			serviceRev := &SvcRevision{
				ServiceName:  latestSvcTmpl.ServiceName,
				Type:         latestSvcTmpl.Type,
				NextRevision: latestSvcTmpl.Revision,
				Updatable:    true,
				New:          true,
			}
			if latestSvcTmpl.Type == setting.K8SDeployType {
				serviceRev.Containers = make([]*commonmodels.Container, 0)
				for _, container := range latestSvcTmpl.Containers {
					serviceRev.Containers = append(serviceRev.Containers, &commonmodels.Container{
						Image:     container.Image,
						Name:      container.Name,
						ImageName: util.GetImageNameFromContainerInfo(container.ImageName, container.Name),
					})
				}
			}
			serviceRevs = append(serviceRevs, serviceRev)
		}
	}

	for _, service := range productServices {
		// services exist in product but not in template services
		// these services can be deleted from product
		if _, ok := serviceTmplMap[service.ServiceName]; !ok {
			serviceRev := &SvcRevision{
				ServiceName: service.ServiceName,
				Updatable:   true,
				Deleted:     true,
				Type:        service.Type,
				Containers:  service.Containers,
				Error:       service.Error,
			}
			serviceRevs = append(serviceRevs, serviceRev)
		} else {
			maxServiceTmpl, ok := latestSvcTmplMap[service.ServiceName]
			if !ok {
				log.Errorf("service %s not found in allServiceTmpls", service.ServiceName)
				return serviceRevs, fmt.Errorf("failed to find service: %s in allServiceTmpls", service.ServiceName)
			}

			serviceRev := &SvcRevision{
				ServiceName:     service.ServiceName,
				Type:            service.Type,
				CurrentRevision: service.Revision,
				Error:           service.Error,
				NextRevision:    maxServiceTmpl.Revision,
			}

			if serviceRev.NextRevision > serviceRev.CurrentRevision {
				serviceRev.Updatable = true
			}

			// 容器化部署方式才需要设置镜像
			// 容器化部署方式才需要对比配置文件
			if service.Type == setting.K8SDeployType {
				serviceRev.Containers = make([]*commonmodels.Container, 0)

				for _, container := range maxServiceTmpl.Containers {
					c := &commonmodels.Container{
						Image:     container.Image,
						Name:      container.Name,
						ImageName: util.GetImageNameFromContainerInfo(container.ImageName, container.Name),
					}

					// reuse existed container image
					for _, exitedContainer := range service.Containers {
						if exitedContainer.Name == container.Name {
							c.Image = exitedContainer.Image
							break
						}
					}
					serviceRev.Containers = append(serviceRev.Containers, c)
				}

				serviceRevs = append(serviceRevs, serviceRev)
			} else if service.Type == setting.HelmDeployType {
				serviceRevs = append(serviceRevs, serviceRev)
			} else if service.Type == setting.PMDeployType {
				serviceRevs = append(serviceRevs, serviceRev)
			}
		}
	}
	return serviceRevs, nil
}
