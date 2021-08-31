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
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/27149chen/afero"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/yaml"

	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	templatemodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models/template"
	commonrepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	templaterepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb/template"
	commonservice "github.com/koderover/zadig/pkg/microservice/aslan/core/common/service"
	"github.com/koderover/zadig/pkg/setting"
	e "github.com/koderover/zadig/pkg/tool/errors"
	"github.com/koderover/zadig/pkg/types"
)

type HelmService struct {
	Services  []*models.Service `json:"services"`
	FileInfos []*types.FileInfo `json:"file_infos"`
}

type HelmServiceReq struct {
	ProductName string   `json:"product_name"`
	CreateBy    string   `json:"create_by"`
	CodehostID  int      `json:"codehost_id"`
	RepoOwner   string   `json:"repo_owner"`
	RepoName    string   `json:"repo_name"`
	BranchName  string   `json:"branch_name"`
	FilePaths   []string `json:"file_paths"`
	Revision    int64    `json:"revision"`
	SrcPath     string   `json:"src_path"`
}

type HelmServiceArgs struct {
	ProductName      string             `json:"product_name"`
	CreateBy         string             `json:"create_by"`
	HelmServiceInfos []*HelmServiceInfo `json:"helm_service_infos"`
}

type HelmServiceInfo struct {
	ServiceName string `json:"service_name"`
	FilePath    string `json:"file_path"`
	FileName    string `json:"file_name"`
	FileContent string `json:"file_content"`
}

type HelmServiceModule struct {
	ServiceModules []*ServiceModule `json:"service_modules"`
	Service        *models.Service  `json:"service,omitempty"`
}

type Chart struct {
	APIVersion string `json:"apiVersion"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
}

func ListHelmServices(productName string, log *zap.SugaredLogger) (*HelmService, error) {
	helmService := &HelmService{
		Services:  []*models.Service{},
		FileInfos: []*types.FileInfo{},
	}

	opt := &commonrepo.ServiceListOption{
		ProductName: productName,
		Type:        setting.HelmDeployType,
	}

	services, err := commonrepo.NewServiceColl().ListMaxRevisions(opt)
	if err != nil {
		log.Errorf("[helmService.list] err:%v", err)
		return nil, e.ErrListTemplate.AddErr(err)
	}
	helmService.Services = services

	if len(services) > 0 {
		fis, err := loadServiceFileInfos(services[0].ProductName, services[0].ServiceName, "")
		if err != nil {
			log.Errorf("Failed to load service file info, err: %s", err)
			return nil, e.ErrListTemplate.AddErr(err)
		}
		helmService.FileInfos = fis
	}
	return helmService, nil
}

func GetHelmServiceModule(serviceName, productName string, revision int64, log *zap.SugaredLogger) (*HelmServiceModule, error) {
	serviceTemplate, err := commonservice.GetServiceTemplate(serviceName, setting.HelmDeployType, productName, setting.ProductStatusDeleting, revision, log)
	if err != nil {
		return nil, err
	}
	helmServiceModule := new(HelmServiceModule)
	serviceModules := make([]*ServiceModule, 0)
	for _, container := range serviceTemplate.Containers {
		serviceModule := new(ServiceModule)
		serviceModule.Container = container
		buildObj, _ := commonrepo.NewBuildColl().Find(&commonrepo.BuildFindOption{ProductName: productName, ServiceName: serviceName, Targets: []string{container.Name}})
		if buildObj != nil {
			serviceModule.BuildName = buildObj.Name
		}
		serviceModules = append(serviceModules, serviceModule)
	}
	helmServiceModule.Service = serviceTemplate
	helmServiceModule.ServiceModules = serviceModules
	return helmServiceModule, err
}

func GetFilePath(serviceName, productName, dir string, _ *zap.SugaredLogger) ([]*types.FileInfo, error) {
	return loadServiceFileInfos(productName, serviceName, dir)
}

func GetFileContent(serviceName, productName, filePath, fileName string, log *zap.SugaredLogger) (string, error) {
	base := config.LocalServicePath(productName, serviceName)

	svc, err := commonrepo.NewServiceColl().Find(&commonrepo.ServiceFindOption{
		ProductName: productName,
		ServiceName: serviceName,
	})
	if err != nil {
		return "", e.ErrFileContent.AddDesc(err.Error())
	}

	err = commonservice.PreLoadServiceManifests(base, svc)
	if err != nil {
		return "", e.ErrFileContent.AddDesc(err.Error())
	}

	file := filepath.Join(base, serviceName, filePath, fileName)
	fileContent, err := os.ReadFile(file)
	if err != nil {
		log.Errorf("Failed to read file %s, err: %s", file, err)
		return "", e.ErrFileContent.AddDesc(err.Error())
	}

	return string(fileContent), nil
}

func CreateOrUpdateHelmService(args *HelmServiceReq, log *zap.SugaredLogger) error {
	helmRenderCharts := make([]*templatemodels.RenderChart, 0, len(args.FilePaths))
	var errs *multierror.Error

	var wg wait.Group
	var mux sync.RWMutex
	for _, p := range args.FilePaths {
		filePath := strings.TrimLeft(p, "/")
		wg.Start(func() {
			var err error
			var containChartYaml, containValuesYaml, containTemplates bool
			var serviceName, valuesYaml, chartVersion string
			var valuesMap map[string]interface{}

			defer func() {
				if err != nil {
					mux.Lock()
					errs = multierror.Append(errs, err)
					mux.Unlock()
				}
			}()

			log.Infof("Loading chart under path %s", filePath)
			fsTree, err := commonservice.DownloadServiceManifestsFromSource(
				&commonservice.DownloadFromSourceParams{CodehostID: args.CodehostID, Owner: args.RepoOwner, Repo: args.RepoName, Path: filePath, Branch: args.BranchName},
				func(chartTree afero.Fs) (string, error) {
					baseDir := filepath.Base(filePath)
					files, err := afero.ReadDir(chartTree, baseDir)
					if err != nil {
						log.Errorf("Failed to read dir %s, err: %s", baseDir, err)
						return "", err
					}

					for _, file := range files {
						if file.Name() == setting.ChartYaml {
							yamlFile, err := afero.ReadFile(chartTree, filepath.Join(baseDir, setting.ChartYaml))
							if err != nil {
								log.Errorf("Failed to read %s, err: %s", setting.ChartYaml, err)
								return "", err
							}
							chart := new(Chart)
							if err = yaml.Unmarshal(yamlFile, chart); err != nil {
								log.Errorf("Failed to unmarshal yaml %s, err: %s", setting.ChartYaml, err)
								return "", err
							}
							serviceName = chart.Name
							chartVersion = chart.Version
							containChartYaml = true
						} else if file.Name() == setting.ValuesYaml {
							yamlFileContent, err := afero.ReadFile(chartTree, filepath.Join(baseDir, setting.ValuesYaml))
							if err != nil {
								log.Errorf("Failed to read %s, err: %s", setting.ValuesYaml, err)
								return "", err
							}

							if err = yaml.Unmarshal(yamlFileContent, &valuesMap); err != nil {
								log.Errorf("Failed to unmarshal yaml %s, err: %s", setting.ValuesYaml, err)
								return "", err
							}

							valuesYaml = string(yamlFileContent)
							containValuesYaml = true
						} else if file.Name() == setting.TemplatesDir {
							containTemplates = true
						}
					}

					return serviceName, nil
				})

			if !containChartYaml || !containValuesYaml || !containTemplates {
				err = e.ErrCreateTemplate.AddDesc(fmt.Sprintf("%s不是合法的chart目录,目录中必须包含%s/%s/%s目录等请检查!", filePath, setting.ValuesYaml, setting.ChartYaml, setting.TemplatesDir))
				return
			}

			log.Infof("Found valid chart, start to loading it as service %s", serviceName)

			helmRenderCharts = append(helmRenderCharts, &templatemodels.RenderChart{
				ServiceName:  serviceName,
				ChartVersion: chartVersion,
				ValuesYaml:   valuesYaml,
			})

			serviceTemplate := fmt.Sprintf(setting.ServiceTemplateCounterName, serviceName, args.ProductName)
			rev, err1 := commonrepo.NewCounterColl().GetNextSeq(serviceTemplate)
			if err1 != nil {
				log.Errorf("Failed to get next revision for service %s, err: %s", serviceName, err1)
				err = e.ErrCreateTemplate.AddErr(err1)
				return
			}
			args.Revision = rev
			if err1 := commonrepo.NewServiceColl().Delete(serviceName, setting.HelmDeployType, args.ProductName, setting.ProductStatusDeleting, args.Revision); err1 != nil {
				log.Warnf("Failed to delete stale service %s with revision %d, err: %s", serviceName, args.Revision, err1)
			}
			containerList := recursionGetImage(valuesMap)
			if len(containerList) == 0 {
				_, containerList = recursionGetImageByColon(valuesMap)
			}
			serviceObj := &models.Service{
				ServiceName: serviceName,
				Type:        setting.HelmDeployType,
				Revision:    rev,
				ProductName: args.ProductName,
				Visibility:  setting.PrivateVisibility,
				CreateTime:  time.Now().Unix(),
				CreateBy:    args.CreateBy,
				Containers:  containerList,
				CodehostID:  args.CodehostID,
				RepoOwner:   args.RepoOwner,
				RepoName:    args.RepoName,
				BranchName:  args.BranchName,
				LoadPath:    filePath,
				SrcPath:     args.SrcPath,
				HelmChart: &models.HelmChart{
					Name:       serviceName,
					Version:    chartVersion,
					ValuesYaml: valuesYaml,
				},
			}

			log.Infof("Starting to create service %s with revision %d", serviceName, rev)

			if err1 := commonrepo.NewServiceColl().Create(serviceObj); err1 != nil {
				log.Errorf("Failed to create service %s error: %s", serviceName, err1)
				err = e.ErrCreateTemplate.AddDesc(err1.Error())
				return
			}

			log.Info("Service created, Starting to save and upload files")

			// save files to disk and upload them to s3
			if err1 = commonservice.SaveAndUploadService(args.ProductName, serviceName, fsTree); err1 != nil {
				log.Errorf("Failed to save or upload files for service %s in project %s, error: %s", args.ProductName, serviceName, err1)
				err = e.ErrCreateTemplate.AddDesc(err1.Error())
				return
			}

			// we need to update the project sequentially
			mux.Lock()
			defer mux.Unlock()

			p, err1 := templaterepo.NewProductColl().Find(args.ProductName)
			if err1 != nil {
				log.Errorf("Failed to save or upload files for service %s in project %s, error: %s", args.ProductName, serviceName, err1)
				err = e.ErrCreateTemplate.AddDesc(err1.Error())
				return
			}

			updated := true
			if len(p.Services) == 0 {
				p.Services = [][]string{{serviceName}}
			} else if !sets.NewString(p.Services[0]...).Has(serviceName) {
				p.Services[0] = append(p.Services[0], serviceName)
			} else {
				updated = false
			}

			if updated {
				log.Infof("Updating project services to %v", p.Services)

				err1 = templaterepo.NewProductColl().Update(args.ProductName, p)
				if err1 != nil {
					log.Errorf("Failed to update project, err: %v", err1)
					err = e.ErrCreateTemplate.AddDesc(err1.Error())
					return
				}
			}
		})
	}

	wg.Wait()

	go func() {
		compareHelmVariable(helmRenderCharts, args, log)
	}()

	return errs.ErrorOrNil()
}

func loadServiceFileInfos(productName, serviceName, dir string) ([]*types.FileInfo, error) {
	base := config.LocalServicePath(productName, serviceName)

	svc, err := commonrepo.NewServiceColl().Find(&commonrepo.ServiceFindOption{
		ProductName: productName,
		ServiceName: serviceName,
	})
	if err != nil {
		return nil, e.ErrFilePath.AddDesc(err.Error())
	}

	err = commonservice.PreLoadServiceManifests(base, svc)
	if err != nil {
		return nil, e.ErrFilePath.AddDesc(err.Error())
	}
	var fis []*types.FileInfo
	files, err := os.ReadDir(filepath.Join(base, serviceName, dir))
	if err != nil {
		return nil, e.ErrFilePath.AddDesc(err.Error())
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

func UpdateHelmService(args *HelmServiceArgs, log *zap.SugaredLogger) error {
	var serviceNames []string
	for _, helmServiceInfo := range args.HelmServiceInfos {
		serviceNames = append(serviceNames, helmServiceInfo.ServiceName)

		opt := &commonrepo.ServiceFindOption{
			ProductName: args.ProductName,
			ServiceName: helmServiceInfo.ServiceName,
			Type:        setting.HelmDeployType,
		}
		preServiceTmpl, err := commonrepo.NewServiceColl().Find(opt)
		if err != nil {
			return e.ErrUpdateTemplate.AddDesc(err.Error())
		}

		base := config.LocalServicePath(args.ProductName, helmServiceInfo.ServiceName)
		if err = commonservice.PreLoadServiceManifests(base, preServiceTmpl); err != nil {
			return e.ErrUpdateTemplate.AddDesc(err.Error())
		}

		filePath := filepath.Join(base, helmServiceInfo.ServiceName, helmServiceInfo.FilePath, helmServiceInfo.FileName)
		if err = os.WriteFile(filePath, []byte(helmServiceInfo.FileContent), 0644); err != nil {
			log.Errorf("Failed to write file %s, err: %s", filePath, err)
			return e.ErrUpdateTemplate.AddDesc(err.Error())
		}

		// TODO：use yaml compare instead of just comparing the characters
		if helmServiceInfo.FileName == setting.ValuesYaml && preServiceTmpl.HelmChart.ValuesYaml != helmServiceInfo.FileContent {
			var valuesMap map[string]interface{}
			if err = yaml.Unmarshal([]byte(helmServiceInfo.FileContent), &valuesMap); err != nil {
				return e.ErrCreateTemplate.AddDesc("values.yaml解析失败")
			}

			containerList := recursionGetImage(valuesMap)
			if len(containerList) == 0 {
				_, containerList = recursionGetImageByColon(valuesMap)
			}
			preServiceTmpl.Containers = containerList
			preServiceTmpl.HelmChart.ValuesYaml = helmServiceInfo.FileContent

			//修改helm renderset
			renderOpt := &commonrepo.RenderSetFindOption{Name: args.ProductName}
			if rs, err := commonrepo.NewRenderSetColl().Find(renderOpt); err == nil {
				for _, chartInfo := range rs.ChartInfos {
					if chartInfo.ServiceName == helmServiceInfo.ServiceName {
						chartInfo.ValuesYaml = helmServiceInfo.FileContent
						break
					}
				}
				if err = commonrepo.NewRenderSetColl().Update(rs); err != nil {
					log.Errorf("[renderset.update] err:%v", err)
				}
			}
		} else if helmServiceInfo.FileName == setting.ChartYaml {
			chart := new(Chart)
			if err = yaml.Unmarshal([]byte(helmServiceInfo.FileContent), chart); err != nil {
				return e.ErrCreateTemplate.AddDesc(fmt.Sprintf("解析%s失败", setting.ChartYaml))
			}
			if preServiceTmpl.HelmChart.Version != chart.Version {
				preServiceTmpl.HelmChart.Version = chart.Version

				//修改helm renderset
				renderOpt := &commonrepo.RenderSetFindOption{Name: args.ProductName}
				if rs, err := commonrepo.NewRenderSetColl().Find(renderOpt); err == nil {
					for _, chartInfo := range rs.ChartInfos {
						if chartInfo.ServiceName == helmServiceInfo.ServiceName {
							chartInfo.ChartVersion = chart.Version
							break
						}
					}
					if err = commonrepo.NewRenderSetColl().Update(rs); err != nil {
						log.Errorf("[renderset.update] err:%v", err)
					}
				}
			}
		}

		preServiceTmpl.CreateBy = args.CreateBy
		serviceTemplate := fmt.Sprintf(setting.ServiceTemplateCounterName, helmServiceInfo.ServiceName, preServiceTmpl.ProductName)
		rev, err := commonrepo.NewCounterColl().GetNextSeq(serviceTemplate)
		if err != nil {
			return fmt.Errorf("get next helm service revision error: %v", err)
		}

		preServiceTmpl.Revision = rev
		if err := commonrepo.NewServiceColl().Delete(helmServiceInfo.ServiceName, setting.HelmDeployType, args.ProductName, setting.ProductStatusDeleting, preServiceTmpl.Revision); err != nil {
			log.Errorf("helmService.update delete %s error: %v", helmServiceInfo.ServiceName, err)
		}

		if err := commonrepo.NewServiceColl().Create(preServiceTmpl); err != nil {
			log.Errorf("helmService.update serviceName:%s error:%v", helmServiceInfo.ServiceName, err)
			return e.ErrUpdateTemplate.AddDesc(err.Error())
		}
	}

	for _, serviceName := range serviceNames {
		if err := commonservice.UploadFilesToS3(args.ProductName, serviceName, os.DirFS(config.LocalServicePath(args.ProductName, serviceName))); err != nil {
			return e.ErrUpdateTemplate.AddDesc(err.Error())
		}
	}

	return nil
}

// compareHelmVariable 比较helm变量是否有改动，是否需要添加新的renderSet
func compareHelmVariable(chartInfos []*templatemodels.RenderChart, args *HelmServiceReq, log *zap.SugaredLogger) {
	// 对比上个版本的renderset，新增一个版本
	latestChartInfos := make([]*templatemodels.RenderChart, 0)
	renderOpt := &commonrepo.RenderSetFindOption{Name: args.ProductName}
	if latestDefaultRenderSet, err := commonrepo.NewRenderSetColl().Find(renderOpt); err == nil {
		latestChartInfos = latestDefaultRenderSet.ChartInfos
	}

	currentChartInfoMap := make(map[string]*templatemodels.RenderChart)
	for _, chartInfo := range chartInfos {
		currentChartInfoMap[chartInfo.ServiceName] = chartInfo
	}

	mixtureChartInfos := make([]*templatemodels.RenderChart, 0)
	for _, latestChartInfo := range latestChartInfos {
		//如果新的里面存在就拿新的数据替换，不存在就还使用老的数据
		if currentChartInfo, isExist := currentChartInfoMap[latestChartInfo.ServiceName]; isExist {
			mixtureChartInfos = append(mixtureChartInfos, currentChartInfo)
			delete(currentChartInfoMap, latestChartInfo.ServiceName)
			continue
		}
		mixtureChartInfos = append(mixtureChartInfos, latestChartInfo)
	}

	//把新增的服务添加到新的slice里面
	for _, chartInfo := range currentChartInfoMap {
		mixtureChartInfos = append(mixtureChartInfos, chartInfo)
	}

	//添加renderset
	if err := commonservice.CreateHelmRenderSet(
		&models.RenderSet{
			Name:        args.ProductName,
			Revision:    0,
			ProductTmpl: args.ProductName,
			UpdateBy:    args.CreateBy,
			ChartInfos:  mixtureChartInfos,
		}, log,
	); err != nil {
		log.Errorf("helmService.Create CreateHelmRenderSet error: %v", err)
	}
}

// 递归通过repository和tag获取服务组件
func recursionGetImage(jsonValues map[string]interface{}) []*models.Container {
	ret := make([]*models.Container, 0)
	for jsonKey, jsonValue := range jsonValues {
		if levelMap, ok := jsonValue.(map[string]interface{}); ok {
			ret = append(ret, recursionGetImage(levelMap)...)
		} else if repository, isStr := jsonValue.(string); isStr {
			if strings.Contains(jsonKey, "repository") {
				serviceContainer := new(models.Container)
				if imageTag, isExist := jsonValues["tag"]; isExist {
					if imageTag != "" {
						serviceContainer.Image = fmt.Sprintf("%s:%v", repository, imageTag)
						imageStr := strings.Split(repository, "/")
						if len(imageStr) > 1 {
							serviceContainer.Name = imageStr[len(imageStr)-1]
						}
						ret = append(ret, serviceContainer)
					}
				}
			}
		}
	}
	return ret
}

func recursionGetImageByColon(jsonValues map[string]interface{}) ([]string, []*models.Container) {
	imageRegEx := regexp.MustCompile(config.ImageRegexString)
	ret := make([]*models.Container, 0)
	banList := sets.NewString()

	for _, jsonValue := range jsonValues {
		if levelMap, ok := jsonValue.(map[string]interface{}); ok {
			imageList, recursiveRet := recursionGetImageByColon(levelMap)
			ret = append(ret, recursiveRet...)
			banList.Insert(imageList...)
		} else if imageName, isStr := jsonValue.(string); isStr {
			if strings.Contains(imageName, ":") && imageRegEx.MatchString(imageName) &&
				!strings.Contains(imageName, "http") && !strings.Contains(imageName, "https") {
				serviceContainer := new(models.Container)
				serviceContainer.Image = imageName
				imageArr := strings.Split(imageName, "/")
				if len(imageArr) >= 1 {
					imageTagArr := strings.Split(imageArr[len(imageArr)-1], ":")
					serviceContainer.Name = imageTagArr[0]
				}
				if !banList.Has(imageName) {
					banList.Insert(imageName)
					ret = append(ret, serviceContainer)
				}
			}
		}
	}
	return banList.List(), ret
}
