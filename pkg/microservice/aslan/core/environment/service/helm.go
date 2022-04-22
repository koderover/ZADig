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
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/otiai10/copy"
	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/releaseutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/informers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models/template"
	commonrepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	commonservice "github.com/koderover/zadig/pkg/microservice/aslan/core/common/service"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/kube"
	"github.com/koderover/zadig/pkg/setting"
	e "github.com/koderover/zadig/pkg/tool/errors"
	helmtool "github.com/koderover/zadig/pkg/tool/helmclient"
	"github.com/koderover/zadig/pkg/tool/kube/serializer"
	"github.com/koderover/zadig/pkg/tool/kube/updater"
	"github.com/koderover/zadig/pkg/tool/log"
	"github.com/koderover/zadig/pkg/types"
	"github.com/koderover/zadig/pkg/util"
	"github.com/koderover/zadig/pkg/util/converter"
)

type HelmReleaseResp struct {
	ReleaseName string `json:"releaseName"`
	ServiceName string `json:"serviceName"`
	Revision    int    `json:"revision"`
	Chart       string `json:"chart"`
	AppVersion  string `json:"appVersion"`
}

type ChartInfo struct {
	ServiceName string `json:"serviceName"`
	Revision    int64  `json:"revision"`
}

type HelmChartsResp struct {
	ChartInfos []*ChartInfo      `json:"chartInfos"`
	FileInfos  []*types.FileInfo `json:"fileInfos"`
}

type ImageData struct {
	ImageName string `json:"imageName"`
	ImageTag  string `json:"imageTag"`
}

type ServiceImages struct {
	ServiceName string       `json:"serviceName"`
	Images      []*ImageData `json:"imageData"`
}

type ChartImagesResp struct {
	ServiceImages []*ServiceImages `json:"serviceImages"`
}

func ListReleases(productName, envName string, log *zap.SugaredLogger) ([]*HelmReleaseResp, error) {
	opt := &commonrepo.ProductFindOptions{Name: productName, EnvName: envName}
	prod, err := commonrepo.NewProductColl().Find(opt)
	if err != nil {
		return nil, e.ErrCreateDeliveryVersion.AddDesc(err.Error())
	}

	restConfig, err := kube.GetRESTConfig(prod.ClusterID)
	if err != nil {
		log.Errorf("GetRESTConfig error: %v", err)
		return nil, e.ErrCreateDeliveryVersion.AddDesc(err.Error())
	}
	helmClient, err := helmtool.NewClientFromRestConf(restConfig, prod.Namespace)
	if err != nil {
		log.Errorf("[%s][%s] NewClientFromRestConf error: %v", envName, productName, err)
		return nil, e.ErrCreateDeliveryVersion.AddErr(err)
	}

	releases, err := helmClient.ListDeployedReleases()
	if err != nil {
		return nil, e.ErrCreateDeliveryVersion.AddErr(err)
	}

	// filter releases, only list releases deployed by zadig
	releaseNameMap, err := commonservice.GetReleaseNameToServiceNameMap(prod)
	if err != nil {
		return nil, err
	}

	ret := make([]*HelmReleaseResp, 0, len(releases))
	for _, release := range releases {
		serviceName, ok := releaseNameMap[release.Name]
		if !ok {
			continue
		}
		ret = append(ret, &HelmReleaseResp{
			ReleaseName: release.Name,
			ServiceName: serviceName,
			Revision:    release.Version,
			Chart:       release.Chart.Name(),
			AppVersion:  release.Chart.AppVersion(),
		})
	}
	return ret, nil
}

func loadChartFilesInfo(productName, serviceName string, revision int64, dir string) ([]*types.FileInfo, error) {
	base := config.LocalServicePathWithRevision(productName, serviceName, revision)

	var fis []*types.FileInfo
	files, err := os.ReadDir(filepath.Join(base, serviceName, dir))
	if err != nil {
		log.Warnf("failed to read chart info for service %s with revision %d", serviceName, revision)
		base = config.LocalServicePath(productName, serviceName)
		files, err = os.ReadDir(filepath.Join(base, serviceName, dir))
		if err != nil {
			return nil, err
		}
	}

	for _, file := range files {
		info, _ := file.Info()
		if info == nil {
			continue
		}
		fi := &types.FileInfo{
			Parent:  dir,
			Name:    file.Name(),
			Size:    info.Size(),
			Mode:    file.Type(),
			ModTime: info.ModTime().Unix(),
			IsDir:   file.IsDir(),
		}

		fis = append(fis, fi)
	}
	return fis, nil
}

//prepare chart version data
func prepareChartVersionData(prod *models.Product, serviceObj *models.Service, renderChart *template.RenderChart, renderset *models.RenderSet) error {
	productName := prod.ProductName
	serviceName, revision := serviceObj.ServiceName, serviceObj.Revision
	base := config.LocalServicePathWithRevision(productName, serviceName, revision)
	if err := commonservice.PreloadServiceManifestsByRevision(base, serviceObj); err != nil {
		log.Warnf("failed to get chart of revision: %d for service: %s, use latest version", revision, serviceName)
		// use the latest version when it fails to download the specific version
		base = config.LocalServicePath(productName, serviceName)
		if err = commonservice.PreLoadServiceManifests(base, serviceObj); err != nil {
			log.Errorf("failed to load chart info for service %v", serviceObj.ServiceName)
			return err
		}
	}

	fullPath := filepath.Join(base, serviceObj.ServiceName)
	deliveryChartPath := filepath.Join(config.LocalDeliveryChartPathWithRevision(productName, serviceObj.ServiceName, serviceObj.Revision), serviceObj.ServiceName)
	err := copy.Copy(fullPath, deliveryChartPath)
	if err != nil {
		return err
	}

	restConfig, err := kube.GetRESTConfig(prod.ClusterID)
	if err != nil {
		log.Errorf("get rest config error: %s", err)
		return err
	}
	helmClient, err := helmtool.NewClientFromRestConf(restConfig, prod.Namespace)
	if err != nil {
		log.Errorf("[%s][%s] init helm client error: %s", prod.EnvName, productName, err)
		return err
	}

	releaseName := util.GeneReleaseName(serviceObj.GetReleaseNaming(), prod.ProductName, prod.Namespace, prod.EnvName, serviceObj.ServiceName)
	valuesMap, err := helmClient.GetReleaseValues(releaseName, true)
	if err != nil {
		log.Errorf("failed to get values map data, err: %s", err)
		return err
	}

	currentValuesYaml, err := yaml.Marshal(valuesMap)
	if err != nil {
		return err
	}

	// write values.yaml
	if err = os.WriteFile(filepath.Join(deliveryChartPath, setting.ValuesYaml), currentValuesYaml, 0644); err != nil {
		return err
	}

	return nil
}

func GetChartInfos(productName, envName, serviceName string, log *zap.SugaredLogger) (*HelmChartsResp, error) {
	opt := &commonrepo.ProductFindOptions{Name: productName, EnvName: envName}
	prod, err := commonrepo.NewProductColl().Find(opt)
	if err != nil {
		return nil, e.ErrGetHelmCharts.AddErr(err)
	}
	renderSet, err := FindHelmRenderSet(productName, prod.Render.Name, log)
	if err != nil {
		log.Errorf("[%s][P:%s] find product renderset error: %v", envName, productName, err)
		return nil, e.ErrGetHelmCharts.AddErr(err)
	}

	chartMap := make(map[string]*template.RenderChart)
	for _, chart := range renderSet.ChartInfos {
		chartMap[chart.ServiceName] = chart
	}

	allServiceMap := prod.GetServiceMap()
	serviceMap := make(map[string]*models.ProductService)

	//validate data, make sure service and chart info exists
	if len(serviceName) > 0 {
		serviceList := strings.Split(serviceName, ",")
		for _, singleService := range serviceList {
			if service, ok := allServiceMap[singleService]; ok {
				serviceMap[service.ServiceName] = service
			} else {
				return nil, e.ErrGetHelmCharts.AddDesc(fmt.Sprintf("failed to find service %s in target namespace", singleService))
			}
		}
	} else {
		serviceMap = allServiceMap
	}

	if len(serviceMap) == 0 {
		return nil, nil
	}

	ret := &HelmChartsResp{
		ChartInfos: make([]*ChartInfo, 0),
		FileInfos:  make([]*types.FileInfo, 0),
	}

	errList := new(multierror.Error)
	wg := sync.WaitGroup{}

	for _, service := range serviceMap {
		ret.ChartInfos = append(ret.ChartInfos, &ChartInfo{
			ServiceName: service.ServiceName,
			Revision:    service.Revision,
		})
		wg.Add(1)
		// download chart info with particular version
		go func(serviceName string, revision int64) {
			defer wg.Done()
			serviceObj, err := commonrepo.NewServiceColl().Find(&commonrepo.ServiceFindOption{
				ProductName: productName,
				ServiceName: serviceName,
				Revision:    revision,
				Type:        setting.HelmDeployType,
			})
			if err != nil {
				log.Errorf("failed to query services name: %s, revision: %d, error: %s", serviceName, revision, err)
				errList = multierror.Append(errList, fmt.Errorf("failed to query service, serviceName: %s, revision: %d", serviceName, revision))
				return
			}
			renderChart, ok := chartMap[serviceName]
			if !ok {
				errList = multierror.Append(errList, fmt.Errorf("failed to find render chart for service %s in target namespace", serviceName))
				return
			}
			err = prepareChartVersionData(prod, serviceObj, renderChart, renderSet)
			if err != nil {
				errList = multierror.Append(errList, fmt.Errorf("failed to prepare chart info for service %s", serviceObj.ServiceName))
				return
			}
		}(service.ServiceName, service.Revision)
	}
	wg.Wait()

	if errList.ErrorOrNil() != nil {
		return nil, errList.ErrorOrNil()
	}

	// expand file info for first service
	serviceToExpand := ret.ChartInfos[0].ServiceName
	fis, err := loadChartFilesInfo(productName, serviceToExpand, serviceMap[serviceToExpand].Revision, "")
	if err != nil {
		log.Errorf("Failed to load service file info, err: %s", err)
		return nil, e.ErrListTemplate.AddErr(err)
	}
	ret.FileInfos = fis

	return ret, nil
}

func GetImageInfos(productName, envName, serviceNames string, log *zap.SugaredLogger) (*ChartImagesResp, error) {
	opt := &commonrepo.ProductFindOptions{Name: productName, EnvName: envName}
	prod, err := commonrepo.NewProductColl().Find(opt)
	if err != nil {
		return nil, fmt.Errorf("failed to find product: %s:%s to get image infos, err: %s", productName, envName, err)
	}

	restConfig, err := kube.GetRESTConfig(prod.ClusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get rest config: %s:%s to get image infos, err: %s", productName, envName, err)
	}
	helmClient, err := helmtool.NewClientFromRestConf(restConfig, prod.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to init kube client: %s:%s to get image infos, err: %s", productName, envName, err)
	}

	// filter releases, only list releases deployed by zadig
	serviceMap := prod.GetServiceMap()
	templateSvcs, err := commonservice.GetProductUsedTemplateSvcs(prod)
	if err != nil {
		return nil, fmt.Errorf("failed to get service tempaltes,  err: %s", err)
	}
	templateSvcMap := make(map[string]*models.Service)
	for _, ts := range templateSvcs {
		templateSvcMap[ts.ServiceName] = ts
	}
	services := strings.Split(serviceNames, ",")

	ret := &ChartImagesResp{}

	for _, svcName := range services {
		prodSvc, ok := serviceMap[svcName]
		if !ok || prodSvc == nil {
			return nil, fmt.Errorf("failed to find service: %s in product", svcName)
		}

		ts, ok := templateSvcMap[svcName]
		if !ok {
			return nil, fmt.Errorf("failed to find template service: %s", svcName)
		}

		releaseName := util.GeneReleaseName(ts.GetReleaseNaming(), productName, prod.Namespace, prod.EnvName, svcName)
		valuesYaml, err := helmClient.GetReleaseValues(releaseName, true)
		if err != nil {
			return nil, fmt.Errorf("failed to get values for relase: %s, err: %s", releaseName, err)
		}

		flatMap, err := converter.Flatten(valuesYaml)
		if err != nil {
			return nil, fmt.Errorf("failed to get flat map url for release :%s", releaseName)
		}

		svcImage := &ServiceImages{
			ServiceName: svcName,
			Images:      nil,
		}

		for _, container := range prodSvc.Containers {
			if container.ImagePath == nil {
				return nil, fmt.Errorf("failed to parse image for container:%s", container.Image)
			}
			imageSearchRule := &template.ImageSearchingRule{
				Repo:  container.ImagePath.Repo,
				Image: container.ImagePath.Image,
				Tag:   container.ImagePath.Tag,
			}
			pattern := imageSearchRule.GetSearchingPattern()
			imageUrl, err := commonservice.GeneImageURI(pattern, flatMap)
			if err != nil {
				return nil, fmt.Errorf("failed to get image url for container:%s", container.Image)
			}

			svcImage.Images = append(svcImage.Images, &ImageData{
				util.GetImageNameFromContainerInfo(container.ImageName, container.Name),
				commonservice.ExtractImageTag(imageUrl),
			})
		}
		ret.ServiceImages = append(ret.ServiceImages, svcImage)
	}
	return ret, nil
}

func helmInitEnvConfigSet(envName, productName, userName string, envConfigYamls []string, inf informers.SharedInformerFactory, kubeClient client.Client) error {
	return initEnvConfigSetAction(envName, productName, userName, envConfigYamls, inf, kubeClient)
}

func initEnvConfigSetAction(envName, productName, userName string, envConfigYamls []string, inf informers.SharedInformerFactory, kubeClient client.Client) error {
	errList := &multierror.Error{
		ErrorFormat: func(es []error) string {
			format := "创建环境配置"
			if len(es) == 1 {
				return fmt.Sprintf(format+" %s 失败:%s", envName, es[0])
			}
			points := make([]string, len(es))
			for i, err := range es {
				points[i] = fmt.Sprintf("* %s", err)
			}
			return fmt.Sprintf(format+" %s 失败:\n%s", envName, strings.Join(points, "\n"))
		},
	}

	clusterLabels := getPredefinedClusterLabels(productName, "", envName)
	delete(clusterLabels, "s-service")
	for _, envConfigYaml := range envConfigYamls {
		manifests := releaseutil.SplitManifests(envConfigYaml)
		resources := make([]*unstructured.Unstructured, 0, len(manifests))
		for _, item := range manifests {
			u, err := serializer.NewDecoder().YamlToUnstructured([]byte(item))
			if err != nil {
				log.Errorf("Failed to convert yaml to Unstructured, manifest is\n%s\n, error: %s", item, err)
				errList = multierror.Append(errList, err)
				continue
			}
			resources = append(resources, u)
		}
		for _, u := range resources {
			switch u.GetKind() {
			case setting.ConfigMap, setting.Ingress, setting.Secret, setting.PersistentVolumeClaim:
				ls := kube.MergeLabels(clusterLabels, u.GetLabels())
				u.SetNamespace(productName + "-env-" + envName)
				u.SetLabels(ls)

				err := updater.CreateOrPatchUnstructuredNeverAnnotation(u, kubeClient)
				if err != nil {
					log.Errorf("Failed to initEnvConfigSet %s, manifest is\n%v\n, error: %s", u.GetKind(), u, err)
					errList = multierror.Append(errList, err)
					continue
				}
				u.SetManagedFields(nil)
				yamlData, err := yaml.Marshal(u.UnstructuredContent())
				if err != nil {
					log.Errorf("Failed to initEnvConfigSet yaml.Marshal %s, manifest is\n%v\n, error: %s", u.GetKind(), u, err)
					errList = multierror.Append(errList, err)
					continue
				}
				switch u.GetKind() {
				case setting.ConfigMap:
					envCm := &models.EnvConfigMap{
						ProductName:    productName,
						UpdateUserName: userName,
						EnvName:        envName,
						Name:           u.GetName(),
						YamlData:       string(yamlData),
					}
					if err := commonrepo.NewConfigMapColl().Create(envCm, true); err != nil {
						errList = multierror.Append(errList, err)
					}
				case setting.Ingress:
					envIngress := &models.EnvIngress{
						ProductName:    productName,
						UpdateUserName: userName,
						EnvName:        envName,
						Name:           u.GetName(),
						YamlData:       string(yamlData),
					}
					if err := commonrepo.NewIngressColl().Create(envIngress, true); err != nil {
						errList = multierror.Append(errList, err)
					}
				case setting.Secret:
					envSecret := &models.EnvSecret{
						ProductName:    productName,
						UpdateUserName: userName,
						EnvName:        envName,
						Name:           u.GetName(),
						YamlData:       string(yamlData),
					}
					if err := commonrepo.NewSecretColl().Create(envSecret, true); err != nil {
						errList = multierror.Append(errList, err)
					}
				case setting.PersistentVolumeClaim:
					envPvc := &models.EnvPvc{
						ProductName:    productName,
						UpdateUserName: userName,
						EnvName:        envName,
						Name:           u.GetName(),
						YamlData:       string(yamlData),
					}
					if err := commonrepo.NewPvcColl().Create(envPvc, true); err != nil {
						errList = multierror.Append(errList, err)
					}
				}
			default:
				errList = multierror.Append(errList, fmt.Errorf("Failed to initEnvConfigSet %s, manifest is\n%v\n, error: %s", u.GetKind(), u, "kind not support"))
			}
		}
	}
	if len(errList.Errors) == 0 {
		return nil
	}
	return errList
}
