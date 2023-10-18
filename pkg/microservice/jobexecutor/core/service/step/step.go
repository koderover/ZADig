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

package step

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/koderover/zadig/pkg/microservice/jobexecutor/config"
	"github.com/koderover/zadig/pkg/microservice/jobexecutor/core/service/cmd"
	"github.com/koderover/zadig/pkg/microservice/jobexecutor/core/service/configmap"
	"github.com/koderover/zadig/pkg/microservice/jobexecutor/core/service/meta"
	"github.com/koderover/zadig/pkg/tool/log"
	"github.com/koderover/zadig/pkg/util"
	"go.uber.org/zap"
)

type Step interface {
	Run(ctx context.Context) error
}

func RunStep(ctx context.Context, metaData *meta.JobMetaData, updater configmap.Updater, logger *zap.SugaredLogger) error {
	var stepInstance Step
	var err error

	switch metaData.Step.StepType {
	case "shell":
		stepInstance, err = NewShellStep(metaData, logger)
		if err != nil {
			return err
		}
	case "git":
		stepInstance, err = NewGitStep(metaData, logger)
		if err != nil {
			return err
		}
	case "docker_build":
		stepInstance, err = NewDockerBuildStep(metaData, logger)
		if err != nil {
			return err
		}
	case "tools":
		stepInstance, err = NewToolInstallStep(metaData, logger)
		if err != nil {
			return err
		}
	case "archive":
		stepInstance, err = NewArchiveStep(metaData, logger)
		if err != nil {
			return err
		}
	case "junit_report":
		stepInstance, err = NewJunitReportStep(metaData, logger)
		if err != nil {
			return err
		}
	case "tar_archive":
		stepInstance, err = NewTararchiveStep(metaData, logger)
		if err != nil {
			return err
		}
	case "sonar_check":
		stepInstance, err = NewSonarCheckStep(metaData, logger)
		if err != nil {
			return err
		}
	case "distribute_image":
		stepInstance, err = NewDistributeImageStep(metaData, logger)
		if err != nil {
			return err
		}
	case "debug_before":
		stepInstance, err = NewDebugStep("before", metaData, updater, logger)
		if err != nil {
			return err
		}
	case "debug_after":
		stepInstance, err = NewDebugStep("after", metaData, updater, logger)
		if err != nil {
			return err
		}
	default:
		err := fmt.Errorf("step type: %s does not match any known type", metaData.Step.StepType)
		log.Error(err)
		return err
	}
	if err := stepInstance.Run(ctx); err != nil {
		log.Error(err)
		return err
	}
	return nil
}

func prepareScriptsEnv() []string {
	scripts := []string{}
	scripts = append(scripts, "eval $(ssh-agent -s) > /dev/null")
	// $HOME/.ssh/id_rsa 为 github 私钥
	scripts = append(scripts, fmt.Sprintf("ssh-add %s/.ssh/id_rsa.github &> /dev/null", config.Home()))
	scripts = append(scripts, fmt.Sprintf("rm %s/.ssh/id_rsa.github &> /dev/null", config.Home()))
	// $HOME/.ssh/gitlab 为 gitlab 私钥
	scripts = append(scripts, fmt.Sprintf("ssh-add %s/.ssh/id_rsa.gitlab &> /dev/null", config.Home()))
	scripts = append(scripts, fmt.Sprintf("rm %s/.ssh/id_rsa.gitlab &> /dev/null", config.Home()))

	return scripts
}

func handleCmdOutput(pipe io.ReadCloser, needPersistentLog bool, logFile string, secretEnvs []string) {
	reader := bufio.NewReader(pipe)

	for {
		lineBytes, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}

			log.Errorf("Failed to read log when processing cmd output: %s", err)
			break
		}

		fmt.Printf("%s", maskSecretEnvs(string(lineBytes), secretEnvs))

		if needPersistentLog {
			err := util.WriteFile(logFile, lineBytes, 0700)
			if err != nil {
				log.Warnf("Failed to write file when processing cmd output: %s", err)
			}
		}
	}
}

const (
	secretEnvMask = "********"
)

func maskSecret(secrets []string, message string) string {
	out := message

	for _, val := range secrets {
		if len(val) == 0 {
			continue
		}
		out = strings.Replace(out, val, "********", -1)
	}
	return out
}

func maskSecretEnvs(message string, secretEnvs []string) string {
	out := message

	for _, val := range secretEnvs {
		if len(val) == 0 {
			continue
		}
		sl := strings.Split(val, "=")

		if len(sl) != 2 {
			continue
		}

		if len(sl[0]) == 0 || len(sl[1]) == 0 {
			// invalid key value pair received
			continue
		}
		out = strings.Replace(out, strings.Join(sl[1:], "="), secretEnvMask, -1)
	}
	return out
}

func isDirEmpty(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return true
	}
	defer f.Close()

	_, err = f.Readdir(1)
	return err == io.EOF
}

func setCmdsWorkDir(dir string, cmds []*cmd.Command) {
	for _, c := range cmds {
		c.Cmd.Dir = dir
	}
}

func makeEnvMap(envs ...[]string) map[string]string {
	envMap := map[string]string{}
	for _, env := range envs {
		for _, env := range env {
			sl := strings.Split(env, "=")
			if len(sl) != 2 {
				continue
			}
			envMap[sl[0]] = sl[1]
		}
	}
	return envMap
}

func SetCmdStdout(cmd *exec.Cmd, fileName string, secretEnvs []string, needPersistentLog bool, wg *sync.WaitGroup) error {
	cmdStdoutReader, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		handleCmdOutput(cmdStdoutReader, needPersistentLog, fileName, secretEnvs)
	}()

	cmdStdErrReader, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		handleCmdOutput(cmdStdErrReader, needPersistentLog, fileName, secretEnvs)
	}()
	return nil
}
