package service

import (
	"net/http"
	"net/url"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/koderover/zadig/pkg/microservice/picket/client/aslan"
	"github.com/koderover/zadig/pkg/microservice/picket/client/opa"
	consts "github.com/koderover/zadig/pkg/microservice/picket/core/const"
)

type rule struct {
	method   string
	endpoint string
}

func ListWorkflows(header http.Header, qs url.Values, logger *zap.SugaredLogger) ([]byte, error) {
	rules := []*rule{{
		method:   "/api/aslan/workflow/workflow",
		endpoint: "GET",
	}}
	names, err := getAllowedProjects(header, rules, consts.AND, logger)
	if err != nil {
		logger.Errorf("Failed to get allowed project names, err: %s", err)
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}
	if !(len(names) == 1 && names[0] == "*") {
		for _, name := range names {
			qs.Add("projects", name)
		}
	}
	return aslan.New().ListWorkflows(header, qs)
}

func ListTestWorkflows(testName string, header http.Header, qs url.Values, logger *zap.SugaredLogger) ([]byte, error) {
	rules := []*rule{{
		method:   "/api/aslan/workflow/workflow",
		endpoint: "PUT",
	}}
	names, err := getAllowedProjects(header, rules, consts.AND, logger)
	if err != nil {
		logger.Errorf("Failed to get allowed project names, err: %s", err)
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}
	if !(len(names) == 1 && names[0] == "*") {
		for _, name := range names {
			qs.Add("projects", name)
		}
	}
	return aslan.New().ListTestWorkflows(testName, header, qs)
}

//getAllowedProjects
//rulesLogicalOperator@ OR:satisfy any one of rules / AND:satisfy all rules
func getAllowedProjects(headers http.Header, rules []*rule, rulesLogicalOperator consts.RulesLogicalOperator, logger *zap.SugaredLogger) (projects []string, err error) {
	var res [][]string
	for _, v := range rules {
		allowedProjects := &allowedProjectsData{}
		opaClient := opa.NewDefault()
		err := opaClient.Evaluate("rbac.user_allowed_projects", allowedProjects, func() (*opa.Input, error) {
			return generateOPAInput(headers, v.method, v.endpoint), nil
		})
		if err != nil {
			logger.Errorf("opa evaluation failed, err: %s", err)
			return nil, err
		}
		res = append(res, allowedProjects.Result)
	}
	if rulesLogicalOperator == consts.OR {
		return union(res), nil
	}
	return intersect(res), nil
}

func union(s [][]string) []string {
	if len(s) == 0 {
		return nil
	}
	tmp := sets.NewString(s[0]...)
	for _, v := range s[1:] {
		t := sets.NewString(v...)
		tmp = t.Union(tmp)
	}
	return tmp.List()
}

func intersect(s [][]string) []string {
	if len(s) == 0 {
		return nil
	}
	tmp := sets.NewString(s[0]...)
	for _, v := range s[1:] {
		t := sets.NewString(v...)
		tmp = t.Intersection(tmp)
	}
	return tmp.List()
}
