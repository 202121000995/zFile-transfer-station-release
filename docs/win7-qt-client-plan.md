# Win7 Qt 专版客户端规划

## 目标

- 运行系统：Windows 7 / Windows 10 / Windows 11
- 运行方式：离线运行，不依赖 WebView2，不要求目标电脑在线安装依赖
- 技术栈：C++ + Qt Widgets + Qt 5.12.x 或 Qt 5.15.x + MinGW
- 界面目标：还原当前 Tauri 版现代蓝白 UI，减少 WebView 冷启动卡顿

## 功能范围

- 上传文件
  - 支持点击选择文件
  - 支持拖拽文件到上传区域
  - 支持上传进度
  - 上传成功显示 5 位取件码

- 下载文件
  - 输入 5 位取件码下载
  - 显示下载进度
  - 下载完成后可自动打开文件夹
  - ZIP 文件可自动解压

- 系统能力
  - 可选开机自启动
  - 本地日志输出，便于定位 Win7 环境问题

## 网络与 TLS

- 使用 Qt Network：
  - `QNetworkAccessManager`
  - `QNetworkRequest`
  - `QHttpMultiPart`
- HTTPS 支持：
  - 随程序打包 OpenSSL DLL
  - 推荐 Win7 兼容 OpenSSL 1.1.1 系列

## 预计体积

- Qt Widgets 原生界面：20MB ~ 45MB
- 加 HTTPS 上传下载：30MB ~ 60MB
- 加自动解压、图标资源、日志：35MB ~ 70MB
- 安装包压缩后：20MB ~ 45MB

## 打包内容

- `文件中转站-win7.exe`
- Qt 运行库 DLL
- MinGW 运行库 DLL
- OpenSSL DLL
- 图标和样式资源
- 可选：安装包或便携 zip

## 不使用

- 不使用 .NET
- 不使用 WebView2
- 不使用 Tauri
- 不使用 Electron
- 不使用浏览器内核

