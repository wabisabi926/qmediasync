# QMediaSync

![GitHub release (latest by date)](https://img.shields.io/github/v/release/qicfan/qmediasync)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/qicfan/qmediasync)

#### 本魔改项目基于个人需求，基于原项目做的精简，仅限于本人使用，不打包成品，请支持原作者，非常简单好用的项目。

## 更新日志：
- 1.精简：搜刮和整理、网盘文件管理、github 升级模块、公告模块、捐赠模块等；
- 2.美化：头像改为 qms 图标；
- 3.修改：使用帮助 超链接官网 https://qmediasync.cn/；
- 4.版本号更新为：0.15.05。

## 讨论方式

- 电报群：[http://t.me/q115_strm](https://t.me/q115_strm)
- QQ群：1057459156
- Meow 官方频道：使用鸿蒙系统手机扫描下方二维码来关注频道（请用官方浏览器打开）
  
  <img src="https://s.mqfamily.top/meow.png" width="200" />

### 开源版本不包含 115 开放平台账号，需要自备

### 本项目接受除了资源（搜索、订阅、下载）、逆向接口的一切功能 PR

- 
## 介绍

- **默认用户名 admin,密码 admin123**
- 默认端口：http-12333   https-12332
- emby 代理端口默认：http-8095  https-8094
- 其他见 [wiki](https://github.com/qicfan/qmediasync/wiki)

## 调试启动

```bash
go run .
```

## 退出

- linux: ```ctrl + c```
- windows: 系统托盘找到 QMediaSync 图标，右键退出

## 编译且发布新版本

```bash
cd build_scripts
sudo ./build_and_release.sh -v vx.xx.xx
```

编译要求具有 github 命令行 gh 权限，且已经登录
如果要发布 docker 镜像，需要提前登录 docker hub
该命令会编译打包所有平台的二进制文件，生成 release 版本，并且发布到 github release 页面，推送到 docker hub（如果要推送到自己的仓库，请修改编译脚本中的用户名和仓库名）

## 数据库

开源版本不包含 postgres 数据库二进制文件，需要自己安装，建议版本 15.x，然后配置环境变量使用。详见 wiki 中的[安装](https://github.com/qicfan/qmediasync/wiki/Linux-%E5%AE%89%E8%A3%85%E4%BD%BF%E7%94%A8)

## 需要自备的密钥

- 115 开放平台 AppID，现在改为使用 OAuth 授权方式，开发者需要根据代码自己实现 OAUTH 服务端来和 115 通信，或者改为二维码扫码登录授权。

全部都在 main.go 文件中开头的变量中设置，也可以在编译时通过 ldflags 传入

## 配套前端

- [QMediaSync-Frontend](https://github.com/qicfan/q115-strm-frontend)

## 贡献者

![Contributors](https://contrib.rocks/image?repo=qicfan/qmediasync)

## Star

![Star History](https://api.star-history.com/svg?repos=qicfan/qmediasync&type=Date)

## 请作者喝杯咖啡

![请作者喝杯咖啡](http://s.mqfamily.top/alipay_wechat.jpg)
