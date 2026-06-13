# GitHub 发布步骤

仓库建议名称：

```text
zFile-transfer-station
```

如果已经安装 Git，并且浏览器已经登录 GitHub，可以按下面步骤发布。

## 1. 初始化本地仓库

```powershell
cd D:\codex\wenjianzhongzhuan
git init
git add .
git commit -m "Initial release"
```

## 2. 在 GitHub 创建空仓库

仓库名建议：

```text
zFile-transfer-station
```

不要勾选自动生成 `README`、`.gitignore` 或 `LICENSE`，因为本地已经有这些文件。

## 3. 绑定远程并推送

把 `<你的GitHub用户名>` 换成你的用户名：

```powershell
git branch -M main
git remote add origin https://github.com/<你的GitHub用户名>/zFile-transfer-station.git
git push -u origin main
```
