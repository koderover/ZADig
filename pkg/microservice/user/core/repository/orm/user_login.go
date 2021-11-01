package orm

import (
	"gorm.io/gorm"

	"github.com/koderover/zadig/pkg/microservice/user/config"
	"github.com/koderover/zadig/pkg/microservice/user/core/repository/models"
)

// CreateUserLogin add a userLogin record
func CreateUserLogin(userLogin *models.UserLogin, db *gorm.DB) error {
	if err := db.Create(&userLogin).Error; err != nil {
		return err
	}
	return nil
}

// GetUserLogin Get a userLogin based on uid
func GetUserLogin(uid string, account string, loginType config.LoginType, db *gorm.DB) (*models.UserLogin, error) {
	var userLogin models.UserLogin
	err := db.Where("uid = ? and login_id = ? and login_type = ?", uid, account, loginType).First(&userLogin).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &userLogin, nil
}

// ListUserLogins Get a userLogin based on uid list
func ListUserLogins(uids []string, db *gorm.DB) (*[]models.UserLogin, error) {
	var userLogins []models.UserLogin
	err := db.Find(&userLogins, "uid in ?", uids).Error
	if err != nil {
		return nil, err
	}
	return &userLogins, nil
}

// UpdateUserLogin update login info
func UpdateUserLogin(uid string, userLogin *models.UserLogin, db *gorm.DB) error {
	if err := db.Model(&models.UserLogin{}).Where("uid = ?", uid).Updates(userLogin).Error; err != nil {
		return err
	}
	return nil
}
