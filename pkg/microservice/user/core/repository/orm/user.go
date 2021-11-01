package orm

import (
	"gorm.io/gorm"

	"github.com/koderover/zadig/pkg/microservice/user/core"
	"github.com/koderover/zadig/pkg/microservice/user/core/repository/models"
)

// CreateUser create a user
func CreateUser(user *models.User, db *gorm.DB) error {
	if err := db.Create(&user).Error; err != nil {
		return err
	}
	return nil
}

// GetUser Get a user based on email and identityType
func GetUser(account string, identityType string, db *gorm.DB) (*models.User, error) {
	var user models.User
	err := db.Where("account = ? and identity_type = ?", account, identityType).First(&user).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &user, nil
}

// GetUserByUid Get a user based on uid
func GetUserByUid(uid string, db *gorm.DB) (*models.User, error) {
	var user models.User
	err := db.Where("uid = ?", uid).First(&user).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &user, nil
}

// ListUsers gets a list of users based on paging constraints
func ListUsers(page int, perPage int, account string, db *gorm.DB) ([]models.User, error) {
	var (
		users []models.User
		err   error
	)

	err = db.Where("account LIKE ?", "%"+account+"%").Offset((page - 1) * perPage).Limit(perPage).Find(&users).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	return users, nil
}

// ListUsersByUIDs gets a list of users based on paging constraints
func ListUsersByUIDs(uids []string, db *gorm.DB) ([]models.User, error) {
	var (
		users []models.User
		err   error
	)

	err = db.Find(&users, "uid in ?", uids).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	return users, nil
}

// GetUsersCount gets user count
func GetUsersCount(account string) (int64, error) {
	var (
		users []models.User
		err   error
		count int64
	)

	err = core.DB.Where("account LIKE ?", "%"+account+"%").Find(&users).Count(&count).Error

	if err != nil {
		return 0, err
	}

	return count, nil
}
