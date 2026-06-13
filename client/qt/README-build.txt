文件中转站 Win7 Qt 专版构建说明

目标：
- Windows 7 SP1 / Windows 10 / Windows 11
- Qt Widgets 原生界面
- 不依赖 .NET / WebView2 / Tauri / Electron
- 离线运行，发布包内包含 Qt DLL、MinGW DLL、OpenSSL DLL

推荐工具链：
- Qt 5.12.x 或 Qt 5.15.x
- MinGW 32-bit 或 MinGW 64-bit
- 若重点照顾低配 Win7，优先 32-bit

构建命令：
1. 打开 Qt 5.x MinGW 命令行
2. 进入目录：
   cd /d D:\codex\wenjianzhongzhuan\client\qt
3. 生成 Makefile：
   qmake FileTransferQt.pro
4. 编译：
   mingw32-make release
5. 部署运行库：
   windeployqt release\transfer-qt-win7.exe

HTTPS 注意：
- Qt Network 访问 https://zz.31pk.top 需要 OpenSSL DLL。
- Qt 5.12 常见依赖：libeay32.dll / ssleay32.dll。
- Qt 5.15 可能依赖：libcrypto-1_1.dll / libssl-1_1.dll。
- DLL 版本必须与 Qt 编译器位数一致。

便携发布目录建议：
- build\client\qt-win7\transfer-qt-win7.exe
- build\client\qt-win7\Qt5Core.dll
- build\client\qt-win7\Qt5Gui.dll
- build\client\qt-win7\Qt5Widgets.dll
- build\client\qt-win7\Qt5Network.dll
- build\client\qt-win7\platforms\qwindows.dll
- build\client\qt-win7\libcrypto / libssl 或 libeay32 / ssleay32

当前实现：
- 点击选择文件
- 拖拽文件上传
- 上传进度
- 显示 5 位取件码
- 输入取件码下载
- 下载进度
- 自动打开下载文件夹
- ZIP 自动解压（调用 Windows Shell COM）
- 开机自启动
- 本地日志：%APPDATA%\zFileTransferStation\文件中转站\transfer-qt.log
