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
	"sort"
	"time"

	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	commonrepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	"github.com/koderover/zadig/pkg/util"
	"go.uber.org/zap"

	models "github.com/koderover/zadig/pkg/microservice/aslan/core/stat/repository/models"
	repo "github.com/koderover/zadig/pkg/microservice/aslan/core/stat/repository/mongodb"
)

type dashboardDeploy struct {
	Total                 int                     `json:"total"`
	Success               int                     `json:"success"`
	DashboardDeployDailys []*dashboardDeployDaily `json:"data"`
}

type dashboardDeployDaily struct {
	Date    string `json:"date"`
	Success int    `json:"success"`
	Failure int    `json:"failure"`
	Total   int    `json:"total"`
}

func GetDeployDailyTotalAndSuccess(args *models.DeployStatOption, log *zap.SugaredLogger) (*dashboardDeploy, error) {
	var (
		dashboardDeploy       = new(dashboardDeploy)
		dashboardDeployDailys = make([]*dashboardDeployDaily, 0)
		failure               int
		success               int
	)

	if deployItems, err := repo.NewDeployStatColl().GetDeployTotalAndSuccess(); err == nil {
		for _, deployItem := range deployItems {
			success += deployItem.TotalSuccess
			failure += deployItem.TotalFailure
		}
		dashboardDeploy.Total = success + failure
		dashboardDeploy.Success = success
	} else {
		log.Errorf("Failed to getDeployTotalAndSuccess err:%s", err)
		return nil, err
	}

	if deployDailyItems, err := repo.NewDeployStatColl().GetDeployDailyTotal(args); err == nil {
		sort.SliceStable(deployDailyItems, func(i, j int) bool { return deployDailyItems[i].Date < deployDailyItems[j].Date })
		for _, deployDailyItem := range deployDailyItems {
			dashboardDeployDaily := new(dashboardDeployDaily)
			dashboardDeployDaily.Date = deployDailyItem.Date
			dashboardDeployDaily.Success = deployDailyItem.TotalSuccess
			dashboardDeployDaily.Failure = deployDailyItem.TotalFailure
			dashboardDeployDaily.Total = deployDailyItem.TotalFailure + deployDailyItem.TotalSuccess

			dashboardDeployDailys = append(dashboardDeployDailys, dashboardDeployDaily)
		}
		dashboardDeploy.DashboardDeployDailys = dashboardDeployDailys
	} else {
		log.Errorf("Failed to getDeployDailyTotal err:%s", err)
		return nil, err
	}

	return dashboardDeploy, nil
}

func GetDeployStats(args *models.DeployStatOption, log *zap.SugaredLogger) (*dashboardDeploy, error) {
	var (
		dashboardDeploy       = new(dashboardDeploy)
		dashboardDeployDailys = make([]*dashboardDeployDaily, 0)
		failure               int
		success               int
	)

	if deployItems, err := repo.NewDeployStatColl().GetDeployStats(&models.DeployStatOption{
		StartDate:    args.StartDate,
		EndDate:      args.EndDate,
		ProductNames: args.ProductNames,
	}); err == nil {
		for _, deployItem := range deployItems {
			success += deployItem.TotalSuccess
			failure += deployItem.TotalFailure
		}
		dashboardDeploy.Total = success + failure
		dashboardDeploy.Success = success
	} else {
		log.Errorf("Failed to getDeployTotalAndSuccess err:%s", err)
		return nil, err
	}

	if deployDailyItems, err := repo.NewDeployStatColl().GetDeployDailyTotal(args); err == nil {
		sort.SliceStable(deployDailyItems, func(i, j int) bool { return deployDailyItems[i].Date < deployDailyItems[j].Date })
		for _, deployDailyItem := range deployDailyItems {
			dashboardDeployDaily := new(dashboardDeployDaily)
			dashboardDeployDaily.Date = deployDailyItem.Date
			dashboardDeployDaily.Success = deployDailyItem.TotalSuccess
			dashboardDeployDaily.Failure = deployDailyItem.TotalFailure
			dashboardDeployDaily.Total = deployDailyItem.TotalFailure + deployDailyItem.TotalSuccess

			dashboardDeployDailys = append(dashboardDeployDailys, dashboardDeployDaily)
		}
		dashboardDeploy.DashboardDeployDailys = dashboardDeployDailys
	} else {
		log.Errorf("Failed to getDeployDailyTotal err:%s", err)
		return nil, err
	}

	return dashboardDeploy, nil
}

type ProjectsDeployStatTotal struct {
	ProjectName     string           `json:"project_name"`
	DeployStatTotal *DeployStatTotal `json:"deploy_stat_total"`
}

type DeployStatTotal struct {
	TotalSuccess int `json:"total_success"`
	TotalFailure int `json:"total_failure"`
	TotalTimeout int `json:"total_timeout"`
}

func GetDeployHealth(start, end int64, projects []string) ([]*ProjectsDeployStatTotal, error) {
	result, err := commonrepo.NewJobInfoColl().GetDeployTrend(start, end, projects)
	if err != nil {
		return nil, err
	}

	stats := make([]*ProjectsDeployStatTotal, 0)
	for _, project := range projects {
		deployStatTotal := &ProjectsDeployStatTotal{
			ProjectName:     project,
			DeployStatTotal: &DeployStatTotal{},
		}
		for _, item := range result {
			if item.ProductName == project {
				switch item.Status {
				case string(config.StatusPassed):
					deployStatTotal.DeployStatTotal.TotalSuccess++
				case string(config.StatusFailed):
					deployStatTotal.DeployStatTotal.TotalFailure++
				case string(config.StatusTimeout):
					deployStatTotal.DeployStatTotal.TotalTimeout++
				}
			}
		}
		stats = append(stats, deployStatTotal)
	}
	return stats, nil
}

type ProjectsWeeklyDeployStat struct {
	Project          string              `json:"project_name"`
	WeeklyDeployStat []*WeeklyDeployStat `json:"weekly_deploy_stat"`
}

type WeeklyDeployStat struct {
	StartTime        string `json:"start_time"`
	Success          int    `json:"success"`
	Failure          int    `json:"failure"`
	Timeout          int    `json:"timeout"`
	AverageBuildTime int    `json:"average_deploy_time"`
}

func GetProjectsDeployWeeklyStat(start, end int64, projects []string) ([]*ProjectsWeeklyDeployStat, error) {
	result, err := commonrepo.NewJobInfoColl().GetDeployTrend(start, end, projects)
	if err != nil {
		return nil, err
	}

	stat := make([]*ProjectsWeeklyDeployStat, 0)
	for _, project := range projects {
		weeklyStat := &ProjectsWeeklyDeployStat{
			Project: project,
		}

		for i := 0; i < len(result); i++ {
			start := util.GetMidnightTimestamp(result[i].StartTime)
			end := time.Unix(start, 0).Add(time.Hour*24*7 - time.Second).Unix()
			deployStat := &WeeklyDeployStat{
				StartTime: time.Unix(start, 0).Format("2006-01-02"),
			}
			count, duration := 0, 0
			for j := i; j < len(result); j++ {
				if project == result[j].ProductName && result[j].StartTime >= start && result[j].StartTime < end {
					switch result[j].Status {
					case string(config.StatusPassed):
						deployStat.Success++
					case string(config.StatusTimeout):
						deployStat.Timeout++
					case string(config.StatusFailed):
						deployStat.Failure++
					}
					count++
					duration += int(result[j].Duration)
				} else {
					deployStat.AverageBuildTime = duration / count
					weeklyStat.WeeklyDeployStat = append(weeklyStat.WeeklyDeployStat, deployStat)
					i = j
					break
				}
			}
		}
		stat = append(stat, weeklyStat)
	}
	return stat, nil
}

type DeployStat struct {
	ProjectName string `json:"project_name"`
	Success     int    `json:"success"`
	Failure     int    `json:"failure"`
	Timeout     int    `json:"timeout"`
	Total       int    `json:"total"`
}

func getDeployStat(start, end int64, project string) (DeployStat, error) {
	result, err := commonrepo.NewJobInfoColl().GetDeployJobs(start, end, project)
	if err != nil {
		return DeployStat{}, err
	}

	resp := DeployStat{
		ProjectName: project,
	}
	for _, job := range result {
		switch job.Status {
		case string(config.StatusPassed):
			resp.Success++
		case string(config.StatusFailed):
			resp.Failure++
		}
	}
	resp.Total = len(result)
	return resp, nil
}
