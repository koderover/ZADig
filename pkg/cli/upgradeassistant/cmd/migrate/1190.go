/*
 * Copyright 2023 The KodeRover Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package migrate

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	internalmodels "github.com/koderover/zadig/pkg/cli/upgradeassistant/internal/repository/models"
	internaldb "github.com/koderover/zadig/pkg/cli/upgradeassistant/internal/repository/mongodb"
	"github.com/koderover/zadig/pkg/cli/upgradeassistant/internal/upgradepath"
	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/kube"
	codehost_mongodb "github.com/koderover/zadig/pkg/microservice/systemconfig/core/codehost/repository/mongodb"
	"github.com/koderover/zadig/pkg/setting"
	"github.com/koderover/zadig/pkg/tool/log"
)

func init() {
	upgradepath.RegisterHandler("1.18.0", "1.19.0", V1180ToV1190)
	upgradepath.RegisterHandler("1.19.0", "1.18.0", V1190ToV1180)
}

func V1180ToV1190() error {
	log.Infof("-------- start migrate cluster workflow schedule strategy --------")
	if err := migrateClusterScheduleStrategy(); err != nil {
		log.Errorf("migrateClusterScheduleStrategy err: %v", err)
		return err
	}

	log.Infof("-------- start migrate workflow template --------")
	if err := migrateWorkflowTemplate(); err != nil {
		log.Infof("migrateWorkflowTemplate err: %v", err)
		return err
	}

	log.Infof("-------- start migrate project management system identity --------")
	if err := migrateProjectManagementSystemIdentity(); err != nil {
		log.Infof("migrateProjectManagementSystemIdentity err: %v", err)
		return err
	}

	log.Infof("-------- start migrate config management system identity --------")
	if err := migrateConfigurationManagementSystemIdentity(); err != nil {
		log.Infof("migrateConfigurationManagementSystemIdentity err: %v", err)
		return err
	}

	log.Infof("-------- start migrate sonar integration system identity --------")
	if err := migrateSonarIntegrationSystemIdentity(); err != nil {
		log.Infof("migrateSonarIntegrationSystemIdentity err: %v", err)
		return err
	}

	log.Infof("-------- start migrate codehost integration level --------")
	if err := migrateCodeHostIntegrationLevel(); err != nil {
		log.Infof("migrateConfigurationManagementSystemIdentity err: %v", err)
		return err
	}

	log.Infof("-------- start migrate sonar scanning --------")
	if err := migrateSonarScanningModules(); err != nil {
		log.Infof("migrate sonar scanning err: %v", err)
		return err
	}

	log.Infof("-------- start migrate apollo --------")
	if err := migrateApolloIntegration(); err != nil {
		log.Infof("migrateApolloIntegration err: %v", err)
		return err
	}

	log.Infof("-------- start migrate infrastructure filed in build & build template module and general job --------")
	if err := migrateInfrastructureField(); err != nil {
		log.Infof("migrate infrastructure filed in build & build template module and general job err: %v", err)
		return err
	}

	return nil
}

func V1190ToV1180() error {
	return nil
}

func migrateClusterScheduleStrategy() error {
	coll := mongodb.NewK8SClusterColl()
	clusters, err := coll.List(nil)
	if err != nil {
		return fmt.Errorf("failed to get all cluster from db, err: %v", err)
	}

	for _, cluster := range clusters {
		if cluster.AdvancedConfig != nil && cluster.AdvancedConfig.ScheduleStrategy != nil {
			continue
		}

		if cluster.AdvancedConfig == nil {
			cluster.AdvancedConfig = &models.AdvancedConfig{
				ClusterAccessYaml: kube.ClusterAccessYamlTemplate,
				ScheduleWorkflow:  true,
				ScheduleStrategy: []*models.ScheduleStrategy{
					{
						StrategyID:   primitive.NewObjectID().Hex(),
						StrategyName: setting.NormalScheduleName,
						Strategy:     setting.NormalSchedule,
						Default:      true,
					},
				},
			}
		} else {
			cluster.AdvancedConfig.ScheduleStrategy = make([]*models.ScheduleStrategy, 0)
			strategy := &models.ScheduleStrategy{
				StrategyID:  primitive.NewObjectID().Hex(),
				Strategy:    cluster.AdvancedConfig.Strategy,
				NodeLabels:  cluster.AdvancedConfig.NodeLabels,
				Tolerations: cluster.AdvancedConfig.Tolerations,
				Default:     true,
			}
			switch strategy.Strategy {
			case setting.NormalSchedule:
				strategy.StrategyName = setting.NormalScheduleName
			case setting.RequiredSchedule:
				strategy.StrategyName = setting.RequiredScheduleName
			case setting.PreferredSchedule:
				strategy.StrategyName = setting.PreferredScheduleName
			}
			cluster.AdvancedConfig.ScheduleStrategy = append(cluster.AdvancedConfig.ScheduleStrategy, strategy)

		}
		err := coll.UpdateScheduleStrategy(cluster)
		if err != nil {
			return fmt.Errorf("failed to update cluster in ua method migrateClusterScheduleStrategy, err: %v", err)
		}
	}
	return nil
}

var oldWorkflowTemplates = []string{
	"业务变更及测试", "数据库及业务变更", "多环境服务变更", "多阶段灰度", "istio发布", "Nacos 配置变更及服务升级", "Apollo 配置变更及服务升级",
}

func migrateWorkflowTemplate() error {
	// delete old workflow templates
	for _, name := range oldWorkflowTemplates {
		query := bson.M{
			"template_name": name,
			"created_by":    setting.SystemUser,
		}
		_, err := mongodb.NewWorkflowV4TemplateColl().DeleteOne(context.TODO(), query)
		if err != nil {
			return fmt.Errorf("failed to delete old workflow template %s for merging custom and release workflow, err: %v", name, err)
		}
	}

	// change release workflow template category to custom workflow template category for merge release and custom workflow
	templateCursor, err := mongodb.NewWorkflowV4TemplateColl().ListByCursor(&mongodb.ListWorkflowV4TemplateOption{Category: setting.ReleaseWorkflow})
	if err != nil {
		return fmt.Errorf("failed to list workflowV4 template for merging custom and release workflow, err: %v", err)
	}
	var ms []mongo.WriteModel
	for templateCursor.Next(context.Background()) {
		var template models.WorkflowV4Template
		if err := templateCursor.Decode(&template); err != nil {
			return err
		}
		if template.Category == setting.ReleaseWorkflow {
			ms = append(ms,
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{"_id", template.ID}}).
					SetUpdate(bson.D{{"$set",
						bson.D{
							{"category", setting.CustomWorkflow},
						}},
					}),
			)
		}
		if len(ms) >= 50 {
			log.Infof("update %d workflowV4 template", len(ms))
			if _, err := mongodb.NewWorkflowV4TemplateColl().BulkWrite(context.Background(), ms); err != nil {
				return fmt.Errorf("update workflowV4 templates for merging custom and release workflow, error: %s", err)
			}
			ms = []mongo.WriteModel{}
		}
	}
	if len(ms) > 0 {
		log.Infof("update %d workflowV4 templates", len(ms))
		if _, err := mongodb.NewWorkflowV4TemplateColl().BulkWrite(context.Background(), ms); err != nil {
			return fmt.Errorf("update workflowV4 templates for merging custom and release workflow, error: %s", err)
		}
	}

	// change release workflow category to custom workflow category for merge release and custom workflow
	cursor, err := mongodb.NewWorkflowV4Coll().ListByCursor(&mongodb.ListWorkflowV4Option{Category: setting.ReleaseWorkflow})
	if err != nil {
		return fmt.Errorf("failed to list workflowV4 for merging custom and release workflow, err: %v", err)
	}
	ms = []mongo.WriteModel{}
	for cursor.Next(context.Background()) {
		var workflow models.WorkflowV4
		if err := cursor.Decode(&workflow); err != nil {
			return err
		}
		if workflow.Category == setting.ReleaseWorkflow {
			ms = append(ms,
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{"_id", workflow.ID}}).
					SetUpdate(bson.D{{"$set",
						bson.D{
							{"category", setting.CustomWorkflow},
						}},
					}),
			)
		}
		if len(ms) >= 50 {
			log.Infof("update %d workflowV4", len(ms))
			if _, err := mongodb.NewWorkflowV4Coll().BulkWrite(context.TODO(), ms); err != nil {
				return fmt.Errorf("update workflowV4s for merging custom and release workflow, error: %s", err)
			}
			ms = []mongo.WriteModel{}
		}
	}
	if len(ms) > 0 {
		log.Infof("update %d workflowV4s", len(ms))
		if _, err := mongodb.NewWorkflowV4Coll().BulkWrite(context.TODO(), ms); err != nil {
			return fmt.Errorf("update workflowV4s for merging custom and release workflow, error: %s", err)
		}
	}
	return nil
}

func migrateProjectManagementSystemIdentity() error {
	// project management system collection
	pms, err := mongodb.NewProjectManagementColl().List()
	if err != nil {
		return fmt.Errorf("failed to list project management, err: %v", err)
	}

	jiraCount := 0
	meegoCount := 0
	for _, pm := range pms {
		if pm.SystemIdentity != "" {
			continue
		}

		systemIdentity := ""
		if pm.Type == setting.PMJira {
			jiraCount++
			systemIdentity = fmt.Sprintf("jira-%d", jiraCount)
		} else if pm.Type == setting.PMMeego {
			meegoCount++
			systemIdentity = fmt.Sprintf("meego-%d", meegoCount)
		}
		pm.SystemIdentity = systemIdentity
		if err := mongodb.NewProjectManagementColl().UpdateByID(pm.ID.Hex(), pm); err != nil {
			return fmt.Errorf("failed to update project management system identity, err: %v", err)
		}
	}

	// workflow jira/meego job
	jira, err := mongodb.NewProjectManagementColl().GetJira()
	if err != nil {
		if mongodb.IsErrNoDocuments(err) {
			jira = nil
		} else {
			return fmt.Errorf("failed to get jira info from project management, err: %v", err)
		}
	}
	meego, err := mongodb.NewProjectManagementColl().GetMeego()
	if err != nil {
		if mongodb.IsErrNoDocuments(err) {
			meego = nil
		} else {
			return fmt.Errorf("failed to get meego info from project management, err: %v", err)
		}
	}
	cursor, err := mongodb.NewWorkflowV4Coll().ListByCursor(&mongodb.ListWorkflowV4Option{
		JobTypes: []config.JobType{config.JobJira, config.JobMeegoTransition},
	})
	if err != nil {
		return fmt.Errorf("failed to list workflowV4 for project management by cursor, err: %v", err)
	}
	var ms []mongo.WriteModel
	for cursor.Next(context.Background()) {
		var workflow models.WorkflowV4
		if err := cursor.Decode(&workflow); err != nil {
			return err
		}

		changed := false
		for _, stage := range workflow.Stages {
			for _, job := range stage.Jobs {
				if job.JobType == config.JobJira {
					spec := &models.JiraJobSpec{}
					if err := models.IToiYaml(job.Spec, spec); err != nil {
						return err
					}

					if spec.JiraID != "" {
						continue
					}

					if jira == nil {
						continue
					}

					spec.JiraID = jira.ID.Hex()
					spec.JiraSystemIdentity = jira.SystemIdentity
					spec.JiraURL = jira.JiraHost

					job.Spec = spec
					changed = true
				} else if job.JobType == config.JobMeegoTransition {
					spec := &models.MeegoTransitionJobSpec{}
					if err := models.IToiYaml(job.Spec, spec); err != nil {
						return err
					}

					if spec.MeegoID != "" {
						continue
					}

					if meego == nil {
						continue
					}

					spec.MeegoID = meego.ID.Hex()
					spec.MeegoSystemIdentity = meego.SystemIdentity
					spec.MeegoURL = meego.MeegoHost

					job.Spec = spec
					changed = true
				}
			}
		}

		if changed {
			ms = append(ms,
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{"_id", workflow.ID}}).
					SetUpdate(bson.D{{"$set",
						bson.D{
							{"stages", workflow.Stages},
						}},
					}),
			)
		}

		if len(ms) >= 50 {
			log.Infof("update %d workflowV4", len(ms))
			if _, err := mongodb.NewWorkflowV4Coll().BulkWrite(context.TODO(), ms); err != nil {
				return fmt.Errorf("update workflowV4s for jira/meego job system identity, error: %s", err)
			}
			ms = []mongo.WriteModel{}
		}
	}
	if len(ms) > 0 {
		log.Infof("update %d workflowV4s", len(ms))
		if _, err := mongodb.NewWorkflowV4Coll().BulkWrite(context.TODO(), ms); err != nil {
			return fmt.Errorf("update workflowV4s for jira/meego job system identity, error: %s", err)
		}
	}

	// workflow meego hook
	query := bson.M{"meego_hook_ctls": bson.M{"$exists": "true", "$ne": bson.A{}}}
	cursor, err = mongodb.NewWorkflowV4Coll().Collection.Find(context.TODO(), query)
	if err != nil {
		return fmt.Errorf("failed to list workflowV4 for meego hook ctls by cursor, err: %v", err)
	}
	ms = []mongo.WriteModel{}
	for cursor.Next(context.Background()) {
		var workflow models.WorkflowV4
		if err := cursor.Decode(&workflow); err != nil {
			return err
		}

		changed := false
		// meego hook
		for _, hook := range workflow.MeegoHookCtls {
			if hook.MeegoID != "" {
				continue
			}

			if meego == nil {
				continue
			}

			hook.MeegoID = meego.ID.Hex()
			hook.MeegoSystemIdentity = meego.SystemIdentity
			hook.MeegoURL = meego.MeegoHost
			changed = true
		}

		if changed {
			ms = append(ms,
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{"_id", workflow.ID}}).
					SetUpdate(bson.D{{"$set",
						bson.D{
							{"meego_hook_ctls", workflow.MeegoHookCtls},
						}},
					}),
			)
		}

		if len(ms) >= 50 {
			log.Infof("update %d workflowV4", len(ms))
			if _, err := mongodb.NewWorkflowV4Coll().BulkWrite(context.TODO(), ms); err != nil {
				return fmt.Errorf("update workflowV4s for meego hook system identity, error: %s", err)
			}
			ms = []mongo.WriteModel{}
		}
	}
	if len(ms) > 0 {
		log.Infof("update %d workflowV4s", len(ms))
		if _, err := mongodb.NewWorkflowV4Coll().BulkWrite(context.TODO(), ms); err != nil {
			return fmt.Errorf("update workflowV4s for meego hook system identity, error: %s", err)
		}
	}

	return nil
}

func migrateConfigurationManagementSystemIdentity() error {
	for _, typeStr := range []string{"apollo", "nacos"} {
		cms, err := mongodb.NewConfigurationManagementColl().List(context.Background(), typeStr)
		if err != nil {
			return fmt.Errorf("failed to list configuration management, err: %v", err)
		}

		count := 0
		for _, cm := range cms {
			if cm.SystemIdentity != "" {
				continue
			}

			count++
			cm.SystemIdentity = fmt.Sprintf("%s-%d", typeStr, count)
			if err := mongodb.NewConfigurationManagementColl().Update(context.Background(), cm.ID.Hex(), cm); err != nil {
				return fmt.Errorf("failed to update configuration management system identity, err: %v", err)
			}
		}
	}

	return nil
}

func migrateSonarIntegrationSystemIdentity() error {
	sonars, _, err := mongodb.NewSonarIntegrationColl().List(context.Background(), 0, 0)
	if err != nil {
		return fmt.Errorf("failed to list sonar integration, err: %v", err)
	}

	count := 0
	for _, sonar := range sonars {
		if sonar.SystemIdentity != "" {
			continue
		}

		count++
		sonar.SystemIdentity = fmt.Sprintf("sonar-%d", count)
		if err := mongodb.NewSonarIntegrationColl().Update(context.Background(), sonar.ID.Hex(), sonar); err != nil {
			return fmt.Errorf("failed to update sonar integration system identity, err: %v", err)
		}
	}

	return nil
}

func migrateCodeHostIntegrationLevel() error {
	if _, err := codehost_mongodb.NewCodehostColl().UpdateMany(context.Background(),
		bson.M{"integration_level": bson.M{
			"$exists": false,
		}},
		bson.M{"$set": bson.M{
			"integration_level": setting.IntegrationLevelSystem,
		}},
	); err != nil {
		return fmt.Errorf("failed to update code host integration level, err: %v", err)
	}

	return nil
}

func migrateSonarScanningModules() error {
	migrationInfo, err := getMigrationInfo()
	if err != nil {
		return fmt.Errorf("failed to get migration info from db, err: %s", err)
	}

	// if the migration hasn't been done, do a migration
	if !migrationInfo.SonarMigration {
		scannings, err := internaldb.NewScanningColl().List(&internaldb.ScanningListOption{Type: "sonarQube"})
		if err != nil {
			return fmt.Errorf("failed to list scannings to migrate, error: %s", err)
		}

		for _, scanning := range scannings {
			scanning.EnableScanner = true
			scanning.AdvancedSetting.Cache = &internalmodels.ScanningCacheSetting{
				CacheEnable: false,
			}
			scanning.Script = scanning.PreScript
			err = internaldb.NewScanningColl().Update(scanning.ID, scanning)
			if err != nil {
				return fmt.Errorf("failed to update scannings, error: %s", err)
			}
		}

		err = internaldb.NewMigrationColl().UpdateMigrationStatus(migrationInfo.ID,
			map[string]interface{}{
				"sonar_migration": true,
			},
		)

		if err != nil {
			return fmt.Errorf("failed to update migration status for sonar scanning migration, error: %s", err)
		}
	}

	return nil
}

func migrateApolloIntegration() error {
	resp, err := mongodb.NewConfigurationManagementColl().List(context.Background(), setting.SourceFromApollo)
	if err != nil {
		return fmt.Errorf("failed to list apollo config, err: %v", err)
	}
	for _, apolloInfo := range resp {
		apolloAuthConfig, err := mongodb.NewConfigurationManagementColl().GetApolloByID(context.Background(), apolloInfo.ID.Hex())
		if err != nil {
			return fmt.Errorf("failed to get apollo config, id %s, err: %v", apolloInfo.ID.Hex(), err)
		}
		if apolloAuthConfig.ApolloAuthConfig.User == "" {
			apolloAuthConfig.ApolloAuthConfig.User = "zadig"
			apolloInfo.AuthConfig = apolloAuthConfig.ApolloAuthConfig
			if err := mongodb.NewConfigurationManagementColl().Update(context.Background(), apolloInfo.ID.Hex(), apolloInfo); err != nil {
				return fmt.Errorf("failed to update apollo config, id %s, err: %v", apolloInfo.ID.Hex(), err)
			}
		}
	}
	return nil
}

func migrateInfrastructureField() error {
	// change build module infrastructure field
	cursor, err := mongodb.NewBuildColl().ListByCursor(&mongodb.BuildListOption{})
	if err != nil {
		return fmt.Errorf("failed to list build module cursor for infrastructure field in migrateInfrastructureField method, err: %v", err)
	}

	var ms []mongo.WriteModel
	for cursor.Next(context.Background()) {
		var build models.Build
		if err := cursor.Decode(&build); err != nil {
			return err
		}

		if build.Infrastructure == "" {
			build.Infrastructure = setting.JobK8sInfrastructure
			ms = append(ms,
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{"_id", build.ID}}).
					SetUpdate(bson.D{{"$set",
						bson.D{
							{"infrastructure", build.Infrastructure},
						}},
					}),
			)
		}

		if len(ms) >= 50 {
			log.Infof("update %d build", len(ms))
			if _, err := mongodb.NewBuildColl().BulkWrite(context.Background(), ms); err != nil {
				return fmt.Errorf("update build for infrastructure field in migrateInfrastructureField method, error: %s", err)
			}
			ms = []mongo.WriteModel{}
		}
	}

	if len(ms) > 0 {
		log.Infof("update %d build", len(ms))
		if _, err := mongodb.NewBuildColl().BulkWrite(context.Background(), ms); err != nil {
			return fmt.Errorf("update build for infrastructure field in migrateInfrastructureField method, error: %s", err)
		}
	}

	// change build template module infrastructure field
	cursor, err = mongodb.NewBuildTemplateColl().ListByCursor(&mongodb.ListBuildTemplateOption{})
	if err != nil {
		return fmt.Errorf("failed to list build template module cursor for infrastructure field in migrateInfrastructureField method, err: %v", err)
	}

	ms = []mongo.WriteModel{}
	for cursor.Next(context.Background()) {
		var buildTemplate models.BuildTemplate
		if err := cursor.Decode(&buildTemplate); err != nil {
			return err
		}

		if buildTemplate.Infrastructure == "" {
			buildTemplate.Infrastructure = setting.JobK8sInfrastructure
			ms = append(ms,
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{"_id", buildTemplate.ID}}).
					SetUpdate(bson.D{{"$set",
						bson.D{
							{"infrastructure", buildTemplate.Infrastructure},
						}},
					}),
			)
		}

		if len(ms) >= 50 {
			log.Infof("update %d build template", len(ms))
			if _, err := mongodb.NewBuildTemplateColl().BulkWrite(context.Background(), ms); err != nil {
				return fmt.Errorf("update build template for infrastructure field in migrateInfrastructureField method, error: %s", err)
			}
			ms = []mongo.WriteModel{}
		}
	}

	if len(ms) > 0 {
		log.Infof("update %d build template", len(ms))
		if _, err := mongodb.NewBuildTemplateColl().BulkWrite(context.Background(), ms); err != nil {
			return fmt.Errorf("update build template for infrastructure field in migrateInfrastructureField method, error: %s", err)
		}
	}

	// change general job module infrastructure field
	cursor, err = mongodb.NewWorkflowV4Coll().ListByCursor(&mongodb.ListWorkflowV4Option{
		JobTypes: []config.JobType{
			config.JobFreestyle,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to list general job module cursor for infrastructure field in migrateInfrastructureField method, err: %v", err)
	}

	ms = []mongo.WriteModel{}
	for cursor.Next(context.Background()) {
		var workflow models.WorkflowV4
		if err := cursor.Decode(&workflow); err != nil {
			return err
		}

		changed := false
		for _, stage := range workflow.Stages {
			for _, job := range stage.Jobs {
				if job.JobType == config.JobFreestyle {
					spec := &models.FreestyleJobSpec{}
					if err := models.IToi(job.Spec, spec); err != nil {
						return err
					}

					if spec.Properties != nil && spec.Properties.Infrastructure == "" {
						spec.Properties.Infrastructure = setting.JobK8sInfrastructure
						job.Spec = spec
						changed = true
					}
				}
			}
		}

		if changed {
			ms = append(ms,
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{"_id", workflow.ID}}).
					SetUpdate(bson.D{{"$set",
						bson.D{
							{"stages", workflow.Stages},
						}},
					}),
			)
		}

		if len(ms) >= 50 {
			log.Infof("update %d workflowV4", len(ms))
			if _, err := mongodb.NewWorkflowV4Coll().BulkWrite(context.Background(), ms); err != nil {
				return fmt.Errorf("update workflowV4 for infrastructure field in migrateInfrastructureField method, error: %s", err)
			}
			ms = []mongo.WriteModel{}
		}
	}
	if len(ms) > 0 {
		log.Infof("update %d workflowV4", len(ms))
		if _, err := mongodb.NewWorkflowV4Coll().BulkWrite(context.Background(), ms); err != nil {
			return fmt.Errorf("update workflowV4 for infrastructure field in migrateInfrastructureField method, error: %s", err)
		}
	}

	return nil
}
