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

package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"

	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	commonmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models/task"
	commonrepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	commonservice "github.com/koderover/zadig/pkg/microservice/aslan/core/common/service"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/base"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/s3"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/webhook"
	workflowservice "github.com/koderover/zadig/pkg/microservice/aslan/core/workflow/service/workflow"
	"github.com/koderover/zadig/pkg/setting"
	"github.com/koderover/zadig/pkg/shared/client/systemconfig"
	e "github.com/koderover/zadig/pkg/tool/errors"
	"github.com/koderover/zadig/pkg/types"
)

func CreateScanningModule(username string, args *Scanning, log *zap.SugaredLogger) error {
	if len(args.Name) == 0 {
		return e.ErrCreateScanningModule.AddDesc("empty Name")
	}

	err := commonservice.ProcessWebhook(args.AdvancedSetting.HookCtl.Items, nil, webhook.ScannerPrefix+args.Name, log)
	if err != nil {
		return e.ErrCreateScanningModule.AddErr(err)
	}

	scanningModule := ConvertToDBScanningModule(args)
	scanningModule.UpdatedBy = username

	err = commonrepo.NewScanningColl().Create(scanningModule)

	if err != nil {
		log.Errorf("Create scanning module %s error: %s", args.Name, err)
		return e.ErrCreateScanningModule.AddErr(err)
	}

	return nil
}

func UpdateScanningModule(id, username string, args *Scanning, log *zap.SugaredLogger) error {
	if len(args.Name) == 0 {
		return e.ErrCreateScanningModule.AddDesc("empty Name")
	}

	scanning, err := commonrepo.NewScanningColl().GetByID(id)
	if err != nil {
		log.Errorf("failed to get scanning information to update webhook, err: %s", err)
		return err
	}

	if scanning.AdvancedSetting.HookCtl.Enabled {
		err = commonservice.ProcessWebhook(args.AdvancedSetting.HookCtl.Items, scanning.AdvancedSetting.HookCtl.Items, webhook.ScannerPrefix+args.Name, log)
		if err != nil {
			log.Errorf("failed to process webhook for scanning: %s, the error is: %s", args.Name, err)
			return e.ErrUpdateScanningModule.AddErr(err)
		}
	} else {
		err = commonservice.ProcessWebhook(args.AdvancedSetting.HookCtl.Items, nil, webhook.ScannerPrefix+args.Name, log)
		if err != nil {
			log.Errorf("failed to process webhook for scanning: %s, the error is: %s", args.Name, err)
			return e.ErrUpdateScanningModule.AddErr(err)
		}
	}

	scanningModule := ConvertToDBScanningModule(args)
	scanningModule.UpdatedBy = username

	err = commonrepo.NewScanningColl().Update(id, scanningModule)

	if err != nil {
		log.Errorf("update scanning module %s error: %s", args.Name, err)
		return e.ErrUpdateScanningModule.AddErr(err)
	}

	return nil
}

func ListScanningModule(projectName string, log *zap.SugaredLogger) ([]*ListScanningRespItem, int64, error) {
	scanningList, total, err := commonrepo.NewScanningColl().List(&commonrepo.ScanningListOption{ProjectName: projectName}, 0, 0)
	if err != nil {
		log.Errorf("failed to list scanning list from mongodb, the error is: %s", err)
		return nil, 0, err
	}
	resp := make([]*ListScanningRespItem, 0)
	for _, scanning := range scanningList {
		res, err := ListScanningTask(scanning.ID.Hex(), 0, 0, log)
		if err != nil {
			log.Errorf("failed to get scanning task statistics, error is: %s", err)
			return nil, 0, err
		}
		var timesTaken int64
		for _, scanTask := range res.ScanTasks {
			timesTaken += scanTask.RunTime
		}
		var avgRuntime int64
		if len(res.ScanTasks) > 0 {
			avgRuntime = timesTaken / int64(len(res.ScanTasks))
		} else {
			avgRuntime = 0
		}
		resp = append(resp, &ListScanningRespItem{
			ID:   scanning.ID.Hex(),
			Name: scanning.Name,
			Statistics: &ScanningStatistic{
				TimesRun:       int64(res.TotalTasks),
				AverageRuntime: avgRuntime,
			},
			CreatedAt: scanning.CreatedAt,
			UpdatedAt: scanning.UpdatedAt,
		})
	}
	return resp, total, nil
}

func GetScanningModuleByID(id string, log *zap.SugaredLogger) (*Scanning, error) {
	scanning, err := commonrepo.NewScanningColl().GetByID(id)
	if err != nil {
		log.Errorf("failed to get scanning from mongodb, the error is: %s", err)
		return nil, err
	}
	return ConvertDBScanningModule(scanning), nil
}

func DeleteScanningModuleByID(id string, log *zap.SugaredLogger) error {
	scanning, err := commonrepo.NewScanningColl().GetByID(id)
	if err != nil {
		log.Errorf("failed to get scanning from mongodb, the error is: %s", err)
		return err
	}

	err = commonservice.ProcessWebhook(nil, scanning.AdvancedSetting.HookCtl.Items, webhook.ScannerPrefix+scanning.Name, log)
	if err != nil {
		log.Errorf("failed to process webhook for scanning module: %s, the error is: %s", id, err)
		return err
	}

	err = commonrepo.NewScanningColl().DeleteByID(id)
	if err != nil {
		log.Errorf("failed to delete scanning from mongodb, the error is: %s", err)
	}
	return err
}

func CreateScanningTask(id string, req []*ScanningRepoInfo, username string, log *zap.SugaredLogger) (int64, error) {
	scanningInfo, err := commonrepo.NewScanningColl().GetByID(id)
	if err != nil {
		log.Errorf("failed to get scanning from mongodb, the error is: %s", err)
		return 0, err
	}

	scanningName := fmt.Sprintf("%s-%s-%s", scanningInfo.Name, id, "scanning-job")

	nextTaskID, err := commonrepo.NewCounterColl().GetNextSeq(fmt.Sprintf(setting.ScanningTaskFmt, scanningName))
	if err != nil {
		log.Errorf("failed to generated task id for scanning task, error: %s", err)
		return 0, e.ErrGetCounter.AddDesc(err.Error())
	}

	imageInfo, err := commonrepo.NewBasicImageColl().Find(scanningInfo.ImageID)
	if err != nil {
		log.Errorf("failed to get image information to create scanning task, error: %s", err)
		return 0, err
	}

	registries, err := commonservice.ListRegistryNamespaces("", true, log)
	if err != nil {
		log.Errorf("ListRegistryNamespaces err:%v", err)
		return 0, err
	}

	repos := make([]*types.Repository, 0)
	for _, arg := range req {
		rep, err := systemconfig.New().GetCodeHost(arg.CodehostID)
		if err != nil {
			log.Errorf("failed to get codehost info from mongodb, the error is: %s", err)
			return 0, err
		}
		repos = append(repos, &types.Repository{
			Source:      rep.Type,
			RepoOwner:   arg.RepoOwner,
			RepoName:    arg.RepoName,
			Branch:      arg.Branch,
			PR:          arg.PR,
			CodehostID:  arg.CodehostID,
			OauthToken:  rep.AccessToken,
			Address:     rep.Address,
			Username:    rep.Username,
			Password:    rep.Password,
			EnableProxy: rep.EnableProxy,
		})
	}

	scanningTask := &task.Scanning{
		TaskType:   config.TaskScanning,
		Status:     config.StatusCreated,
		ScanningID: scanningInfo.ID.Hex(),
		Name:       scanningInfo.Name,
		ImageInfo:  imageInfo.Value,
		ResReq:     scanningInfo.AdvancedSetting.ResReq,
		ResReqSpec: scanningInfo.AdvancedSetting.ResReqSpec,
		Registries: registries,
		Parameter:  scanningInfo.Parameter,
		Script:     scanningInfo.Script,
		Timeout:    DefaultScanningTimeout,
		ClusterID:  scanningInfo.AdvancedSetting.ClusterID,
		Repos:      repos,
	}

	if scanningInfo.ScannerType == "sonarQube" {
		sonarInfo, err := commonrepo.NewSonarIntegrationColl().GetByID(context.TODO(), scanningInfo.SonarID)
		if err != nil {
			log.Errorf("failed to get sonar integration information to create scanning task, error: %s", err)
			return 0, err
		}

		scanningTask.SonarInfo = &types.SonarInfo{
			Token:         sonarInfo.Token,
			ServerAddress: sonarInfo.ServerAddress,
		}
	}

	proxies, err := commonrepo.NewProxyColl().List(&commonrepo.ProxyArgs{})
	if err != nil {
		log.Errorf("failed to get proxy info to create scanning task, error: %s", err)
		return 0, err
	}
	if len(proxies) != 0 {
		scanningTask.Proxy = proxies[0]
	}

	scanningSubtask, err := scanningTask.ToSubTask()
	if err != nil {
		log.Errorf("failed to convert scanning subtask, error: %s", err)
		return 0, e.ErrCreateTask.AddDesc(err.Error())
	}

	stages := make([]*commonmodels.Stage, 0)
	workflowservice.AddSubtaskToStage(&stages, scanningSubtask, scanningInfo.Name)
	sort.Sort(workflowservice.ByStageKind(stages))

	configPayload := commonservice.GetConfigPayload(0)

	defaultS3, err := s3.FindDefaultS3()
	if err != nil {
		log.Errorf("cannot find the default s3 to store the logs, error: %s", err)
		return 0, e.ErrFindDefaultS3Storage.AddDesc("default storage is required by distribute task")
	}

	defaultURL, err := defaultS3.GetEncryptedURL()
	if err != nil {
		log.Errorf("cannot convert the s3 config to an encrypted URI, error: %s", err)
		return 0, e.ErrS3Storage.AddErr(err)
	}

	finalTask := &task.Task{
		TaskID:        nextTaskID,
		ProductName:   scanningInfo.ProjectName,
		PipelineName:  scanningName,
		Type:          config.ScanningType,
		Status:        config.StatusCreated,
		TaskCreator:   username,
		CreateTime:    time.Now().Unix(),
		Stages:        stages,
		ConfigPayload: configPayload,
		StorageURI:    defaultURL,
	}

	if len(finalTask.Stages) <= 0 {
		return 0, e.ErrCreateTask.AddDesc(e.PipelineSubTaskNotFoundErrMsg)
	}

	if err := workflowservice.CreateTask(finalTask); err != nil {
		log.Error(err)
		return 0, e.ErrCreateTask
	}

	return nextTaskID, nil
}

func ListScanningTask(id string, pageNum, pageSize int64, log *zap.SugaredLogger) (*ListScanningTaskResp, error) {
	scanningInfo, err := commonrepo.NewScanningColl().GetByID(id)
	if err != nil {
		log.Errorf("failed to get scanning from mongodb, the error is: %s", err)
		return nil, err
	}

	scanningName := fmt.Sprintf("%s-%s-%s", scanningInfo.Name, id, "scanning-job")
	listTaskOpt := &commonrepo.ListTaskOption{
		PipelineName: scanningName,
		Limit:        int(pageSize),
		Skip:         int((pageNum - 1) * pageSize),
		Detail:       true,
		Type:         config.ScanningType,
	}
	countTaskOpt := &commonrepo.CountTaskOption{
		PipelineNames: []string{scanningName},
		Type:          config.ScanningType,
	}
	resp, err := commonrepo.NewTaskColl().List(listTaskOpt)
	if err != nil {
		log.Errorf("failed to list scanning task for scanning: %s, the error is: %s", id, err)
		return nil, err
	}
	cnt, err := commonrepo.NewTaskColl().Count(countTaskOpt)
	if err != nil {
		log.Errorf("failed to count scanning task for scanning: %s, the error is: %s", id, err)
		return nil, err
	}

	scanTasks := make([]*ScanningTaskResp, 0)

	for _, scanningTask := range resp {
		taskInfo := &ScanningTaskResp{
			ScanID:    scanningTask.TaskID,
			Status:    string(scanningTask.Status),
			Creator:   scanningTask.TaskCreator,
			CreatedAt: scanningTask.CreateTime,
		}
		if scanningTask.Status == config.StatusPassed || scanningTask.Status == config.StatusCancelled || scanningTask.Status == config.StatusFailed {
			taskInfo.RunTime = scanningTask.EndTime - scanningTask.StartTime
		}
		scanTasks = append(scanTasks, taskInfo)
	}

	return &ListScanningTaskResp{
		ScanInfo: &ScanningInfo{
			Editor:    scanningInfo.UpdatedBy,
			UpdatedAt: scanningInfo.UpdatedAt,
		},
		ScanTasks:  scanTasks,
		TotalTasks: cnt,
	}, nil
}

func GetScanningTaskInfo(scanningID string, taskID int64, log *zap.SugaredLogger) (*ScanningTaskDetail, error) {
	scanningInfo, err := commonrepo.NewScanningColl().GetByID(scanningID)
	if err != nil {
		log.Errorf("failed to get scanning from mongodb, the error is: %s", err)
		return nil, err
	}

	scanningName := fmt.Sprintf("%s-%s-%s", scanningInfo.Name, scanningID, "scanning-job")
	resp, err := commonrepo.NewTaskColl().Find(taskID, scanningName, config.ScanningType)
	if err != nil {
		log.Errorf("failed to get task information, error: %s", err)
		return nil, err
	}

	sonarInfo, err := commonrepo.NewSonarIntegrationColl().GetByID(context.TODO(), scanningInfo.SonarID)
	if err != nil {
		log.Errorf("failed to get sonar integration info, error: %s", err)
		return nil, err
	}

	repoInfo := resp.Stages[0].SubTasks[scanningInfo.Name]
	scanningTaskInfo, err := base.ToScanning(repoInfo)
	if err != nil {
		log.Errorf("failed to convert the content into scanning subtask, the error is: %s", err)
		return nil, fmt.Errorf("failed to convert the content into scanning subtask, the error is: %s", err)
	}

	// for security reasons, we set all sensitive information to empty
	for _, repo := range scanningTaskInfo.Repos {
		repo.OauthToken = ""
		repo.Password = ""
		repo.Username = ""
	}

	return &ScanningTaskDetail{
		Creator:    resp.TaskCreator,
		Status:     string(resp.Status),
		CreateTime: resp.CreateTime,
		EndTime:    resp.EndTime,
		RepoInfo:   scanningTaskInfo.Repos,
		ResultLink: sonarInfo.ServerAddress,
	}, nil
}
