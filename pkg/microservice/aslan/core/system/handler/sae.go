/*
Copyright 2023 The KodeRover Authors.

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
	"fmt"

	"github.com/gin-gonic/gin"

	commonmodels "github.com/koderover/zadig/v2/pkg/microservice/aslan/core/common/repository/models"
	sae "github.com/koderover/zadig/v2/pkg/microservice/aslan/core/common/service/sae"
	internalhandler "github.com/koderover/zadig/v2/pkg/shared/handler"
	e "github.com/koderover/zadig/v2/pkg/tool/errors"
)

// @Summary List SAE
// @Description List SAE
// @Tags 	system
// @Accept 	json
// @Produce json
// @Param 	encryptedKey		query		string				true	"encrypted key"
// @Success 200 	{array} 	models.SAE
// @Router /api/aslan/system/sae [get]
func ListSAE(c *gin.Context) {
	ctx, err := internalhandler.NewContextWithAuthorization(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	if err != nil {
		ctx.Err = fmt.Errorf("authorization Info Generation failed: err %s", err)
		ctx.UnAuthorized = true
		return
	}

	encryptedKey := c.Query("encryptedKey")
	if len(encryptedKey) == 0 {
		ctx.Err = e.ErrInvalidParam
		return
	}

	// authorization checks
	if !ctx.Resources.IsSystemAdmin {
		ctx.UnAuthorized = true
		return
	}

	ctx.Resp, ctx.Err = sae.ListSAE(encryptedKey, ctx.Logger)
}

// @Summary List SAE Detail
// @Description List SAE Detail
// @Tags 	system
// @Accept 	json
// @Produce json
// @Success 200 	{array} 	models.SAE
// @Router /api/aslan/system/sae/detail [get]
func ListSAEInfo(c *gin.Context) {
	ctx, err := internalhandler.NewContextWithAuthorization(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	if err != nil {
		ctx.Err = fmt.Errorf("authorization Info Generation failed: err %s", err)
		ctx.UnAuthorized = true
		return
	}

	ctx.Resp, ctx.Err = sae.ListSAEInfo(ctx.Logger)
}

// @Summary Create SAE
// @Description Create SAE
// @Tags 	system
// @Accept 	json
// @Produce json
// @Param 	body 	body 		models.SAE			true 	"body"
// @Success 200
// @Router /api/aslan/system/sae [post]
func CreateSAE(c *gin.Context) {
	ctx, err := internalhandler.NewContextWithAuthorization(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	if err != nil {
		ctx.Err = fmt.Errorf("authorization Info Generation failed: err %s", err)
		ctx.UnAuthorized = true
		return
	}

	// authorization checks
	if !ctx.Resources.IsSystemAdmin {
		ctx.UnAuthorized = true
		return
	}

	args := new(commonmodels.SAE)
	if err := c.BindJSON(args); err != nil {
		ctx.Err = e.ErrInvalidParam.AddDesc("invalid sae json args")
		return
	}

	args.UpdateBy = ctx.UserName
	ctx.Err = sae.CreateSAE(args, ctx.Logger)
}

// @Summary Get SAE
// @Description Get SAE
// @Tags 	system
// @Accept 	json
// @Produce json
// @Success 200 	{object} 	models.SAE
// @Router /api/aslan/system/sae/{id} [get]
func GetSAE(c *gin.Context) {
	ctx, err := internalhandler.NewContextWithAuthorization(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	if err != nil {
		ctx.Err = fmt.Errorf("authorization Info Generation failed: err %s", err)
		ctx.UnAuthorized = true
		return
	}

	// authorization checks
	if !ctx.Resources.IsSystemAdmin {
		ctx.UnAuthorized = true
		return
	}

	id := c.Param("id")
	if len(id) == 0 {
		ctx.Err = e.ErrInvalidParam.AddDesc("invalid id")
		return
	}

	ctx.Resp, ctx.Err = sae.FindSAE(id, "")
}

// @Summary Update SAE
// @Description Update SAE
// @Tags 	system
// @Accept 	json
// @Produce json
// @Param 	id		path		string				true	"sae id"
// @Param 	body 	body 		models.SAE			true 	"body"
// @Success 200
// @Router /api/aslan/system/sae/{id} [put]
func UpdateSAE(c *gin.Context) {
	ctx, err := internalhandler.NewContextWithAuthorization(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	if err != nil {
		ctx.Err = fmt.Errorf("authorization Info Generation failed: err %s", err)
		ctx.UnAuthorized = true
		return
	}

	// authorization checks
	if !ctx.Resources.IsSystemAdmin {
		ctx.UnAuthorized = true
		return
	}

	id := c.Param("id")
	if len(id) == 0 {
		ctx.Err = e.ErrInvalidParam.AddDesc("invalid id")
		return
	}

	args := new(commonmodels.SAE)
	if err := c.BindJSON(args); err != nil {
		ctx.Err = e.ErrInvalidParam.AddDesc("invalid sae json args")
		return
	}
	args.UpdateBy = ctx.UserName

	ctx.Err = sae.UpdateSAE(id, args, ctx.Logger)
}

// @Summary Delete SAE
// @Description Delete SAE
// @Tags 	system
// @Accept 	json
// @Produce json
// @Param 	id		path		string				true	"sae id"
// @Success 200
// @Router /api/aslan/system/sae/{id} [delete]
func DeleteSAE(c *gin.Context) {
	ctx, err := internalhandler.NewContextWithAuthorization(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	if err != nil {
		ctx.Err = fmt.Errorf("authorization Info Generation failed: err %s", err)
		ctx.UnAuthorized = true
		return
	}

	// authorization checks
	if !ctx.Resources.IsSystemAdmin {
		ctx.UnAuthorized = true
		return
	}

	id := c.Param("id")
	if len(id) == 0 {
		ctx.Err = e.ErrInvalidParam.AddDesc("invalid id")
		return
	}

	ctx.Err = sae.DeleteSAE(id)
}

// @Summary Validate SAE
// @Description Validate SAE
// @Tags 	system
// @Accept 	json
// @Produce json
// @Param 	body 	body 		models.SAE			true 	"body"
// @Success 200
// @Router /api/aslan/system/sae/validate [post]
func ValidateSAE(c *gin.Context) {
	ctx, err := internalhandler.NewContextWithAuthorization(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	if err != nil {
		ctx.Err = fmt.Errorf("authorization Info Generation failed: err %s", err)
		ctx.UnAuthorized = true
		return
	}

	// authorization checks
	if !ctx.Resources.IsSystemAdmin {
		ctx.UnAuthorized = true
		return
	}

	args := new(commonmodels.SAE)
	if err := c.BindJSON(args); err != nil {
		ctx.Err = e.ErrInvalidParam.AddDesc("invalid sae json args")
		return
	}

	ctx.Err = sae.ValidateSAE(args)
}
