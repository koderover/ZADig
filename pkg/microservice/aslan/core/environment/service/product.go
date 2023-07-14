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
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	commonmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	templatemodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models/template"
	commonrepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	templaterepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb/template"
	commonservice "github.com/koderover/zadig/pkg/microservice/aslan/core/common/service"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/collaboration"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/kube"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/repository"
	commontypes "github.com/koderover/zadig/pkg/microservice/aslan/core/common/types"
	"github.com/koderover/zadig/pkg/setting"
	e "github.com/koderover/zadig/pkg/tool/errors"
	"github.com/koderover/zadig/pkg/tool/log"
	"github.com/koderover/zadig/pkg/types"
	"github.com/koderover/zadig/pkg/util"
)

var DefaultCleanWhiteList = []string{"spockadmin"}

func CleanProductCronJob(requestID string, log *zap.SugaredLogger) {

	log.Info("[CleanProductCronJob] started ...")
	defer log.Info("[CleanProductCronJob] end")

	products, err := commonrepo.NewProductColl().List(&commonrepo.ProductListOptions{
		Production: util.GetBoolPointer(false),
	})
	if err != nil {
		log.Errorf("[Product.List] error: %v", err)
		return
	}
	envCMMap, err := collaboration.GetEnvCMMap([]string{}, log)
	if err != nil {
		return
	}
	wl := sets.NewString(DefaultCleanWhiteList...)
	wl.Insert(config.CleanSkippedList()...)
	for _, product := range products {
		if wl.Has(product.EnvName) {
			continue
		}

		if product.RecycleDay == 0 {
			continue
		}
		if _, ok := envCMMap[collaboration.BuildEnvCMMapKey(product.ProductName, product.EnvName)]; ok {
			continue
		}
		if time.Now().Unix()-product.UpdateTime > int64(60*60*24*product.RecycleDay) {
			//title := "系统清理产品信息"
			//content := fmt.Sprintf("环境 [%s] 已经连续%d天没有使用, 系统已自动删除该环境, 如有需要请重新创建。", product.EnvName, product.RecycleDay)

			if err := DeleteProduct("robot", product.EnvName, product.ProductName, requestID, true, log); err != nil {
				log.Errorf("[%s][P:%s] delete product error: %v", product.EnvName, product.ProductName, err)

				// 如果有错误，重试删除
				if err := DeleteProduct("robot", product.EnvName, product.ProductName, requestID, true, log); err != nil {
					//content = fmt.Sprintf("系统自动清理环境 [%s] 失败，请手动删除环境。", product.ProductName)
					log.Errorf("[%s][P:%s] retry delete product error: %v", product.EnvName, product.ProductName, err)
				}
			}

			log.Warnf("[%s] product %s deleted", product.EnvName, product.ProductName)
		}
	}
}

func GetInitProduct(productTmplName string, envType types.EnvType, isBaseEnv bool, baseEnvName string, production bool, log *zap.SugaredLogger) (*commonmodels.Product, error) {
	ret := &commonmodels.Product{}

	prodTmpl, err := templaterepo.NewProductColl().Find(productTmplName)
	if err != nil {
		errMsg := fmt.Sprintf("[ProductTmpl.Find] %s error: %v", productTmplName, err)
		log.Error(errMsg)
		return nil, e.ErrGetProduct.AddDesc(errMsg)
	}
	if prodTmpl.IsHelmProduct() {
		err = commonservice.FillProductTemplateValuesYamls(prodTmpl, production, log)
	}
	if err != nil {
		errMsg := fmt.Sprintf("[ProductTmpl.FillProductTemplate] %s error: %v", productTmplName, err)
		log.Error(errMsg)
		return nil, e.ErrGetProduct.AddDesc(errMsg)
	}

	ret.ProductName = prodTmpl.ProductName
	ret.Revision = prodTmpl.Revision
	ret.Services = [][]*commonmodels.ProductService{}
	ret.UpdateBy = prodTmpl.UpdateBy
	ret.CreateTime = prodTmpl.CreateTime
	ret.Render = &commonmodels.RenderInfo{Name: "", Description: ""}
	ret.ServiceRenders = prodTmpl.ChartInfos
	if prodTmpl.IsCVMProduct() {
		ret.Source = setting.PMDeployType
	}

	svcGroupNames := prodTmpl.Services
	if production {
		svcGroupNames = prodTmpl.ProductionServices
	}

	if envType == types.ShareEnv && !isBaseEnv {
		// At this point the request is from the environment share.
		svcGroupNames, err = GetEnvServiceList(context.TODO(), productTmplName, baseEnvName)
		if err != nil {
			return nil, fmt.Errorf("failed to get service list from env %s of product %s: %s", baseEnvName, productTmplName, err)
		}

		// Note: In the Helm scenario, filter `chart_infos` which is used by front-end.
		if prodTmpl.ProductFeature != nil && prodTmpl.ProductFeature.DeployType == setting.HelmDeployType {
			var chartInfos []*templatemodels.ServiceRender
			for _, chartInfo := range ret.ServiceRenders {
				found := false
				for _, svcGroupName := range svcGroupNames {
					if util.InStringArray(chartInfo.ServiceName, svcGroupName) {
						found = true
						break
					}
				}

				if found {
					chartInfos = append(chartInfos, chartInfo)
				}
			}

			ret.ServiceRenders = chartInfos
		}
	}

	for _, names := range svcGroupNames {
		servicesResp := make([]*commonmodels.ProductService, 0)

		for _, serviceName := range names {

			opt := &commonrepo.ServiceFindOption{
				ServiceName:   serviceName,
				ProductName:   productTmplName,
				ExcludeStatus: setting.ProductStatusDeleting,
			}

			serviceTmpl, err := repository.QueryTemplateService(opt, production)
			if err != nil {
				errMsg := fmt.Sprintf("Can not find service with option when creating init projects %+v, error: %v", opt, err)
				log.Error(errMsg)
				continue
			}

			serviceResp := &commonmodels.ProductService{
				ServiceName: serviceTmpl.ServiceName,
				ProductName: serviceTmpl.ProductName,
				Type:        serviceTmpl.Type,
				Revision:    serviceTmpl.Revision,
			}
			if serviceTmpl.Type == setting.K8SDeployType || serviceTmpl.Type == setting.HelmDeployType {
				serviceResp.Containers = make([]*commonmodels.Container, 0)
				for _, c := range serviceTmpl.Containers {
					container := &commonmodels.Container{
						Name:      c.Name,
						Image:     c.Image,
						ImagePath: c.ImagePath,
						ImageName: util.GetImageNameFromContainerInfo(c.ImageName, c.Name),
					}
					serviceResp.Containers = append(serviceResp.Containers, container)
					serviceResp.VariableYaml = serviceTmpl.VariableYaml
					serviceResp.VariableKVs = commontypes.ServiceToRenderVariableKVs(serviceTmpl.ServiceVariableKVs)
				}
			}
			servicesResp = append(servicesResp, serviceResp)
		}
		ret.Services = append(ret.Services, servicesResp)
	}

	return ret, err
}

func GetProduct(username, envName, productName string, log *zap.SugaredLogger) (*ProductResp, error) {
	opt := &commonrepo.ProductFindOptions{Name: productName, EnvName: envName}
	prod, err := commonrepo.NewProductColl().Find(opt)
	if err != nil {
		log.Errorf("[User:%s][EnvName:%s][Product:%s] Product.FindByOwner error: %s", username, envName, productName, err)
		return nil, e.ErrGetEnv
	}

	if len(prod.RegistryID) == 0 {
		reg, _, err := commonservice.FindDefaultRegistry(false, log)
		if err != nil {
			log.Errorf("[User:%s][EnvName:%s][Product:%s] FindDefaultRegistry error: %s", username, envName, productName, err)
			return nil, err
		}
		prod.RegistryID = reg.ID.Hex()
	}
	resp := buildProductResp(prod.EnvName, prod, log)
	return resp, nil
}

func normalStatus(status string) bool {
	if status == setting.PodRunning || status == setting.PodSucceeded {
		return true
	}
	if status == setting.ServiceStatusNoSuspended || status == setting.ServiceStatusAllSuspended || status == setting.ServiceStatusPartSuspended {
		return true
	}
	return false
}

func buildProductResp(envName string, prod *commonmodels.Product, log *zap.SugaredLogger) *ProductResp {
	prodResp := &ProductResp{
		ID:              prod.ID.Hex(),
		ProductName:     prod.ProductName,
		Namespace:       prod.Namespace,
		Services:        [][]string{},
		Status:          setting.PodUnstable,
		EnvName:         prod.EnvName,
		UpdateTime:      prod.UpdateTime,
		UpdateBy:        prod.UpdateBy,
		Render:          prod.Render,
		Error:           prod.Error,
		IsPublic:        prod.IsPublic,
		IsExisted:       prod.IsExisted,
		ClusterID:       prod.ClusterID,
		RecycleDay:      prod.RecycleDay,
		Source:          prod.Source,
		RegisterID:      prod.RegistryID,
		ShareEnvEnable:  prod.ShareEnv.Enable,
		ShareEnvIsBase:  prod.ShareEnv.IsBase,
		ShareEnvBaseEnv: prod.ShareEnv.BaseEnv,
	}

	if prod.ClusterID != "" {
		clusterService, err := kube.NewService(config.HubServerAddress())
		if err != nil {
			prodResp.Status = setting.ClusterNotFound
			prodResp.Error = "未找到该环境绑定的集群"
			return prodResp
		}
		cluster, err := clusterService.GetCluster(prod.ClusterID, log)
		if err != nil {
			prodResp.Status = setting.ClusterNotFound
			prodResp.Error = "未找到该环境绑定的集群"
			return prodResp
		}
		prodResp.IsProd = cluster.Production
		prodResp.ClusterName = cluster.Name
		prodResp.IsLocal = cluster.Local

		if !prodResp.IsLocal && !clusterService.ClusterConnected(prod.ClusterID) && cluster.Type != setting.KubeConfigClusterType {
			prodResp.Status = setting.ClusterDisconnected
			prodResp.Error = "集群未连接"
			return prodResp
		}
	} else {
		prodResp.IsLocal = true
	}

	if prod.Source != setting.SourceFromExternal {
		prodResp.Services = prod.GetGroupServiceNames()
	}

	if prod.Status == setting.ProductStatusCreating {
		prodResp.Status = setting.PodCreating
		return prodResp
	}
	if prod.Status == setting.ProductStatusUpdating {
		prodResp.Status = setting.PodUpdating
		return prodResp
	}
	if prod.Status == setting.ProductStatusDeleting {
		prodResp.Status = setting.PodDeleting
		return prodResp
	}
	if prod.Status == setting.ProductStatusUnknown {
		prodResp.Status = setting.ClusterUnknown
		return prodResp
	}

	var (
		servicesResp = make([]*commonservice.ServiceResp, 0)
		errObj       error
	)

	switch prod.Source {
	case setting.SourceFromExternal, setting.SourceFromHelm:
		_, servicesResp, errObj = commonservice.ListWorkloadsInEnv(envName, prod.ProductName, "", 0, 0, log)
		if len(servicesResp) == 0 && errObj == nil {
			prodResp.Status = prod.Status
			prodResp.Error = prod.Error
			return prodResp
		}
		allRunning := true
		for _, serviceResp := range servicesResp {
			if serviceResp.Type == setting.K8SDeployType && serviceResp.WorkLoadType != setting.CronJob && !normalStatus(serviceResp.Status) {
				allRunning = false
				break
			}
		}
		//TODO is it reasonable to ignore error when all pods are running？
		if allRunning {
			prodResp.Status = setting.PodRunning
			prodResp.Error = ""
		}
	default:
		prodResp.Status, errObj = CalculateProductStatus(prod, log)
		prodResp.Error = ""
	}

	if errObj != nil {
		prodResp.Error = errObj.Error()
	}
	return prodResp
}

func CleanProducts() {
	logger := log.SugaredLogger()

	products, err := commonrepo.NewProductColl().List(&commonrepo.ProductListOptions{
		Production: util.GetBoolPointer(false),
	})
	if err != nil {
		logger.Errorf("ListProducts error: %v\n", err)
		return
	}

	for _, prod := range products {
		_, err := templaterepo.NewProductColl().Find(prod.ProductName)
		if err != nil && err.Error() == "not found" {
			logger.Errorf("环境所属的项目不存在，准备删除此环境, namespace:%s, 项目:%s\n", prod.Namespace, prod.ProductName)
			err = DeleteProduct("CleanProducts", prod.EnvName, prod.ProductName, "", true, logger)
			if err != nil {
				logger.Errorf("delete product failed, namespace:%s, err:%v\n", prod.Namespace, err)
				continue
			}
		}
	}
}

func ResetProductsStatus() {
	products, err := commonrepo.NewProductColl().List(&commonrepo.ProductListOptions{})
	if err != nil {
		fmt.Printf("ResetProductsStatus error: %v\n", err)
		return
	}

	for _, prod := range products {

		if prod.Status == setting.ProductStatusCreating || prod.Status == setting.ProductStatusUpdating || prod.Status == setting.ProductStatusDeleting {
			if err := commonrepo.NewProductColl().UpdateStatus(prod.EnvName, prod.ProductName, setting.ProductStatusFailed); err != nil {
				fmt.Printf("update product status error: %v\n", err)
			}
		}
	}
}
