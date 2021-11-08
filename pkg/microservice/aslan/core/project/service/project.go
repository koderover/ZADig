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
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	templaterepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb/template"
)

type QueryVerbosity string

const (
	VerbosityDetailed QueryVerbosity = "detailed" // all information
	VerbosityBrief    QueryVerbosity = "brief"    // short information or a summary
	VerbosityMinimal  QueryVerbosity = "minimal"  // very little information, usually only a resource identifier

	allProjects = "*"
)

type ProjectListOptions struct {
	IgnoreNoEnvs bool
	Verbosity    QueryVerbosity
	Names        []string
}

type ProjectDetailedRepresentation struct {
	*ProjectBriefRepresentation
	Alias     string `json:"alias"`
	Desc      string `json:"desc"`
	UpdatedAt int64  `json:"updatedAt"`
	UpdatedBy string `json:"updatedBy"`
	Onboard   bool   `json:"onboard"`
	Public    bool   `json:"public"`
}

type ProjectBriefRepresentation struct {
	*ProjectMinimalRepresentation
	Envs []string `json:"envs"`
}

type ProjectMinimalRepresentation struct {
	Name string `json:"name"`
}

func (o *ProjectListOptions) InNames() []string {
	if len(o.Names) == 0 || (len(o.Names) == 1 && o.Names[0] == allProjects) {
		return []string{}
	}

	return o.Names
}

func ListProjects(opts *ProjectListOptions, logger *zap.SugaredLogger) (interface{}, error) {
	switch opts.Verbosity {
	case VerbosityDetailed:
		return listDetailedProjectInfos(opts, logger)
	case VerbosityBrief:
		return listBriefProjectInfos(opts, logger)
	case VerbosityMinimal:
		return listMinimalProjectInfos(opts, logger)
	default:
		return listMinimalProjectInfos(opts, logger)
	}
}

func listDetailedProjectInfos(opts *ProjectListOptions, logger *zap.SugaredLogger) ([]*ProjectDetailedRepresentation, error) {
	var res []*ProjectDetailedRepresentation

	nameSet, nameMap, err := getProjects(opts)
	if err != nil {
		logger.Errorf("Failed to list projects, err: %s", err)
		return nil, err
	}

	nameWithEnvSet, nameWithEnvMap, err := getProjectsWithEnvs(opts)
	if err != nil {
		logger.Errorf("Failed to list projects, err: %s", err)
		return nil, err
	}

	desiredSet := nameSet
	if opts.IgnoreNoEnvs {
		desiredSet = nameSet.Intersection(nameWithEnvSet)
	}

	for name := range desiredSet {
		info := nameMap[name]
		res = append(res, &ProjectDetailedRepresentation{
			ProjectBriefRepresentation: &ProjectBriefRepresentation{
				ProjectMinimalRepresentation: &ProjectMinimalRepresentation{Name: name},
				Envs:                         nameWithEnvMap[name],
			},
			Alias:     info.Alias,
			Desc:      info.Desc,
			UpdatedAt: info.UpdatedAt,
			UpdatedBy: info.UpdatedBy,
			Onboard:   info.OnboardStatus != 0,
			Public:    info.Public,
		})
	}

	return res, nil
}

func listBriefProjectInfos(opts *ProjectListOptions, logger *zap.SugaredLogger) ([]*ProjectBriefRepresentation, error) {
	var res []*ProjectBriefRepresentation

	nameSet, _, err := getProjects(opts)
	if err != nil {
		logger.Errorf("Failed to list projects, err: %s", err)
		return nil, err
	}

	nameWithEnvSet, nameWithEnvMap, err := getProjectsWithEnvs(opts)
	if err != nil {
		logger.Errorf("Failed to list projects, err: %s", err)
		return nil, err
	}

	desiredSet := nameSet
	if opts.IgnoreNoEnvs {
		desiredSet = nameSet.Intersection(nameWithEnvSet)
	}

	for name := range desiredSet {
		res = append(res, &ProjectBriefRepresentation{
			ProjectMinimalRepresentation: &ProjectMinimalRepresentation{Name: name},
			Envs:                         nameWithEnvMap[name],
		})
	}

	return res, nil
}

func listMinimalProjectInfos(opts *ProjectListOptions, logger *zap.SugaredLogger) ([]*ProjectMinimalRepresentation, error) {
	var res []*ProjectMinimalRepresentation

	names, err := templaterepo.NewProductColl().ListNames(opts.InNames())
	if err != nil {
		logger.Errorf("Failed to list project names, err: %s", err)
		return nil, err
	}

	if !opts.IgnoreNoEnvs {
		for _, name := range names {
			res = append(res, &ProjectMinimalRepresentation{Name: name})
		}

		return res, nil
	}

	nameSet := sets.NewString()
	for _, name := range names {
		nameSet.Insert(name)
	}

	nameWithEnvSet, _, err := getProjectsWithEnvs(opts)
	if err != nil {
		logger.Errorf("Failed to list projects, err: %s", err)
		return nil, err
	}

	for name := range nameWithEnvSet {
		// nameWithEnvs may contain projects which are already deleted.
		if !nameSet.Has(name) {
			continue
		}
		res = append(res, &ProjectMinimalRepresentation{Name: name})
	}

	return res, nil
}

func getProjectsWithEnvs(opts *ProjectListOptions) (sets.String, map[string][]string, error) {
	nameWithEnvs, err := mongodb.NewProductColl().ListProjectsInNames(opts.InNames())
	if err != nil {
		return nil, nil, err
	}

	nameSet := sets.NewString()
	nameMap := make(map[string][]string)
	for _, nameWithEnv := range nameWithEnvs {
		nameSet.Insert(nameWithEnv.ProjectName)
		nameMap[nameWithEnv.ProjectName] = nameWithEnv.Envs
	}

	return nameSet, nameMap, nil
}

func getProjects(opts *ProjectListOptions) (sets.String, map[string]*templaterepo.ProjectInfo, error) {
	res, err := templaterepo.NewProductColl().ListProjectBriefs(opts.InNames())
	if err != nil {
		return nil, nil, err
	}

	nameSet := sets.NewString()
	nameMap := make(map[string]*templaterepo.ProjectInfo)
	for _, r := range res {
		nameSet.Insert(r.Name)
		nameMap[r.Name] = r
	}

	return nameSet, nameMap, nil
}
