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

package updater

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateOrPatchUnstructured(u *unstructured.Unstructured, cl client.Client) error {
	return createOrPatchObject(u, cl)
}

func PatchUnstructured(u *unstructured.Unstructured, patchBytes []byte, patchType types.PatchType, cl client.Client) error {
	return createOrPatchObject(u, cl)
}

func CreateOrPatchUnstructuredNeverAnnotation(u *unstructured.Unstructured, cl client.Client) error {
	return createOrPatchObjectNeverAnnotation(u, cl)
}

func UpdateOrCreateUnstructured(u *unstructured.Unstructured, cl client.Client) error {
	return updateOrCreateObject(u, cl)
}

func DeleteUnstructured(u *unstructured.Unstructured, cl client.Client) error {
	return deleteObjectWithDefaultOptions(u, cl)
}
