package models

import "Q115-STRM/internal/db"

// FnosProxyConfig 飞牛影视反代配置
type FnosProxyConfig struct {
	BaseModel
	Enabled  int    `json:"enabled" gorm:"default:0"`          // 是否启用，0-禁用，1-启用
	FnosURL  string `json:"fnos_url" gorm:"type:varchar(500)"`  // 飞牛影视地址
	Port     string `json:"port" gorm:"type:varchar(10)"`       // 反代端口
	PathMaps string `json:"path_maps" gorm:"type:text"`         // STRM路径映射，每行一个：飞牛路径|本地路径
}

func (*FnosProxyConfig) TableName() string {
	return "fnos_proxy_config"
}

var GlobalFnosProxyConfig *FnosProxyConfig

// GetFnosProxyConfig 获取飞牛反代配置
func GetFnosProxyConfig() (*FnosProxyConfig, error) {
	if GlobalFnosProxyConfig != nil {
		return GlobalFnosProxyConfig, nil
	}
	// 检查表是否存在，不存在则自动创建（容错，防止迁移遗漏）
	if !db.Db.Migrator().HasTable(&FnosProxyConfig{}) {
		if err := db.Db.AutoMigrate(&FnosProxyConfig{}); err != nil {
			return nil, err
		}
	}
	config := &FnosProxyConfig{}
	// 首次使用时自动创建默认记录
	if err := db.Db.FirstOrCreate(config).Error; err != nil {
		return nil, err
	}
	GlobalFnosProxyConfig = config
	return GlobalFnosProxyConfig, nil
}

// Update 更新配置
func (c *FnosProxyConfig) Update(updates map[string]interface{}) error {
	if err := db.Db.Model(c).Updates(updates).Error; err != nil {
		return err
	}
	// 刷新全局缓存
	GlobalFnosProxyConfig = c
	return nil
}
