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

package handler

import (
	_ "embed"
	"fmt"

	"sigs.k8s.io/yaml"

	"github.com/koderover/zadig/pkg/shared/client/policy"
	"github.com/koderover/zadig/pkg/tool/log"
)

//go:embed policy.yaml
var policyDefinitions []byte

//go:embed system-policies.yaml
var policiesDefinitions []byte

func (*Router) Policies() []*policy.PolicyMeta {
	res := &policy.PolicyMeta{}
	err := yaml.Unmarshal(policyDefinitions, res)
	if err != nil {
		// should not have happened here
		log.DPanic(err)
	}

	res2 := []*policy.PolicyMeta{}
	err = yaml.Unmarshal(policiesDefinitions, &res2)
	if err != nil {
		// should not have happened here
		log.DPanic(err)
	}
	fmt.Println(len(res2))
	res2 = append(res2, res)

	return res2
}
