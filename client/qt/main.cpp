#include <QtWidgets>
#include <QtNetwork>

static const char *kConfigGroup = "client";
static const char *kServerConfigKey = "serverUrl";

static QString text(const char *value) {
    return QString::fromUtf8(value);
}

static void appendLog(const QString &message) {
    const QString dirPath = QStandardPaths::writableLocation(QStandardPaths::AppDataLocation);
    QDir().mkpath(dirPath);
    QFile file(QDir(dirPath).filePath("transfer-qt.log"));
    if (!file.open(QIODevice::Append | QIODevice::Text)) {
        return;
    }
    QTextStream stream(&file);
    stream.setCodec("UTF-8");
    stream << QDateTime::currentDateTime().toString(Qt::ISODate) << "  " << message << "\n";
}

static QString safeFileName(QString name) {
    if (name.trimmed().isEmpty()) {
        return "download.bin";
    }
    const QString invalid = "<>:\"/\\|?*";
    for (const QChar character : invalid) {
        name.replace(character, "_");
    }
    name = name.trimmed();
    while (name.endsWith('.')) {
        name.chop(1);
    }
    return name.isEmpty() ? QStringLiteral("download.bin") : name;
}

static QString uniquePath(const QString &dirPath, const QString &fileName) {
    QFileInfo fileInfo(fileName);
    const QString baseName = fileInfo.completeBaseName().isEmpty() ? QStringLiteral("download") : fileInfo.completeBaseName();
    const QString suffix = fileInfo.suffix().isEmpty() ? QString() : QStringLiteral(".") + fileInfo.suffix();
    QString candidate = QDir(dirPath).filePath(baseName + suffix);
    int index = 1;
    while (QFileInfo::exists(candidate)) {
        candidate = QDir(dirPath).filePath(QStringLiteral("%1 (%2)%3").arg(baseName).arg(index).arg(suffix));
        ++index;
    }
    return candidate;
}

static QString fileNameFromReply(QNetworkReply *reply, const QString &fallback) {
    QString fileName = QString::fromUtf8(reply->rawHeader("X-File-Name"));
    if (!fileName.isEmpty()) {
        return safeFileName(fileName);
    }

    const QString disposition = QString::fromUtf8(reply->rawHeader("Content-Disposition"));
    const int filenameIndex = disposition.indexOf("filename=");
    if (filenameIndex >= 0) {
        fileName = disposition.mid(filenameIndex + 9).trimmed();
        fileName.remove('"');
        return safeFileName(fileName);
    }

    return safeFileName(fallback);
}

class DropZone : public QFrame {
    Q_OBJECT

public:
    explicit DropZone(QWidget *parent = nullptr) : QFrame(parent) {
        setObjectName("dropZone");
        setAcceptDrops(true);
        setCursor(Qt::PointingHandCursor);
        setFixedHeight(150);

        auto *layout = new QVBoxLayout(this);
        layout->setContentsMargins(16, 22, 16, 12);
        layout->setSpacing(7);

        iconLabel = new QLabel(QStringLiteral("⇧"), this);
        iconLabel->setObjectName("dropIcon");
        iconLabel->setAlignment(Qt::AlignCenter);

        titleLabel = new QLabel(text("点击或拖拽文件到此处上传"), this);
        titleLabel->setObjectName("dropTitle");
        titleLabel->setAlignment(Qt::AlignCenter);

        subtitleLabel = new QLabel(text("支持单个或多个文件（最大 2GB）"), this);
        subtitleLabel->setObjectName("hintText");
        subtitleLabel->setAlignment(Qt::AlignCenter);

        fileLabel = new QLabel(text("未选择文件"), this);
        fileLabel->setObjectName("fileHint");
        fileLabel->setAlignment(Qt::AlignCenter);
        fileLabel->hide();

        layout->addWidget(iconLabel);
        layout->addWidget(titleLabel);
        layout->addWidget(subtitleLabel);
        layout->addWidget(fileLabel);
    }

    void setFiles(const QStringList &filePaths) {
        if (filePaths.isEmpty()) {
            fileLabel->hide();
            return;
        }
        if (filePaths.size() == 1) {
            fileLabel->setText(text("文件：") + QFileInfo(filePaths.first()).fileName());
        } else {
            fileLabel->setText(text("已选择 ") + QString::number(filePaths.size()) + text(" 个文件"));
        }
        fileLabel->show();
    }

signals:
    void clicked();
    void filesDropped(const QStringList &filePaths);

protected:
    void mousePressEvent(QMouseEvent *event) override {
        if (event->button() == Qt::LeftButton) {
            emit clicked();
        }
        QFrame::mousePressEvent(event);
    }

    void dragEnterEvent(QDragEnterEvent *event) override {
        if (!event->mimeData()->hasUrls()) {
            return;
        }
        setProperty("dragging", true);
        refreshStyle();
        event->acceptProposedAction();
    }

    void dragLeaveEvent(QDragLeaveEvent *event) override {
        Q_UNUSED(event)
        setProperty("dragging", false);
        refreshStyle();
    }

    void dropEvent(QDropEvent *event) override {
        setProperty("dragging", false);
        refreshStyle();

        QStringList filePaths;
        for (const QUrl &url : event->mimeData()->urls()) {
            if (url.isLocalFile()) {
                const QString localPath = url.toLocalFile();
                if (QFileInfo(localPath).isFile()) {
                    filePaths << localPath;
                }
            }
        }
        if (!filePaths.isEmpty()) {
            emit filesDropped(filePaths);
        }
    }

private:
    QLabel *iconLabel = nullptr;
    QLabel *titleLabel = nullptr;
    QLabel *subtitleLabel = nullptr;
    QLabel *fileLabel = nullptr;

    void refreshStyle() {
        style()->unpolish(this);
        style()->polish(this);
        update();
    }
};

class TransferWindow : public QWidget {
    Q_OBJECT

public:
    explicit TransferWindow(QWidget *parent = nullptr) : QWidget(parent) {
        setObjectName("mainWindow");
        setWindowTitle(text("文件中转站 v1.0 - Win7 Qt 专版"));
        setFixedSize(420, 517);
        setAcceptDrops(false);

        networkManager = new QNetworkAccessManager(this);
        buildUi();
        applyStyle();
        switchPage(0);
        appendLog("application started");
        QTimer::singleShot(150, this, [this]() {
            ensureServerConfigured();
        });
    }

private:
    QNetworkAccessManager *networkManager = nullptr;
    QStringList selectedFiles;
    QString lastPickupCode;
    QString activeDownloadPath;
    QFile *activeDownloadFile = nullptr;

    QPushButton *sendTab = nullptr;
    QPushButton *receiveTab = nullptr;
    QStackedWidget *pages = nullptr;

    DropZone *dropZone = nullptr;
    QPushButton *uploadButton = nullptr;
    QProgressBar *uploadProgress = nullptr;
    QFrame *resultCard = nullptr;
    QLabel *pickupCodeLabel = nullptr;
    QPushButton *copyButton = nullptr;

    QLineEdit *codeEdit = nullptr;
    QPushButton *downloadButton = nullptr;
    QProgressBar *downloadProgress = nullptr;
    QLabel *statusTitle = nullptr;
    QLabel *statusSub = nullptr;
    QCheckBox *openFolderCheck = nullptr;
    QCheckBox *unzipCheck = nullptr;
    QCheckBox *startupCheck = nullptr;
    QPushButton *settingsButton = nullptr;

    void buildUi() {
        auto *rootLayout = new QVBoxLayout(this);
        rootLayout->setContentsMargins(0, 0, 0, 0);
        rootLayout->setSpacing(0);

        auto *tabs = new QWidget(this);
        tabs->setObjectName("tabBar");
        tabs->setFixedHeight(70);
        auto *tabLayout = new QHBoxLayout(tabs);
        tabLayout->setContentsMargins(24, 0, 24, 0);
        tabLayout->setSpacing(16);

        sendTab = makeTab(text("发送文件"));
        receiveTab = makeTab(text("接收文件"));
        tabLayout->addWidget(sendTab);
        tabLayout->addWidget(receiveTab);
        rootLayout->addWidget(tabs);

        pages = new QStackedWidget(this);
        pages->setFixedHeight(403);
        pages->addWidget(buildSendPage());
        pages->addWidget(buildReceivePage());
        rootLayout->addWidget(pages);
        rootLayout->addWidget(buildFooter());

        connect(sendTab, &QPushButton::clicked, this, [this]() { switchPage(0); });
        connect(receiveTab, &QPushButton::clicked, this, [this]() { switchPage(1); });
    }

    QPushButton *makeTab(const QString &label) {
        auto *button = new QPushButton(label, this);
        button->setObjectName("tabButton");
        button->setCursor(Qt::PointingHandCursor);
        button->setFlat(true);
        button->setFixedHeight(70);
        return button;
    }

    QWidget *buildSendPage() {
        auto *page = new QWidget(this);
        auto *layout = new QVBoxLayout(page);
        layout->setContentsMargins(24, 18, 24, 0);
        layout->setSpacing(16);

        dropZone = new DropZone(page);
        layout->addWidget(dropZone);

        uploadButton = new QPushButton(text("开始上传文件"), page);
        uploadButton->setObjectName("primaryButton");
        uploadButton->setCursor(Qt::PointingHandCursor);
        uploadButton->setFixedHeight(40);
        layout->addWidget(uploadButton);

        uploadProgress = makeProgress(page);
        uploadProgress->hide();
        layout->addWidget(uploadProgress);

        resultCard = new QFrame(page);
        resultCard->setObjectName("resultCard");
        resultCard->setFixedHeight(120);
        auto *resultLayout = new QVBoxLayout(resultCard);
        resultLayout->setContentsMargins(18, 12, 18, 10);
        resultLayout->setSpacing(4);

        auto *resultTitle = new QLabel(text("上传结果"), resultCard);
        resultTitle->setObjectName("resultTitle");
        auto *resultBody = new QLabel(text("上传成功！您的取件码是："), resultCard);
        resultBody->setObjectName("bodyText");

        auto *codeRow = new QHBoxLayout();
        pickupCodeLabel = new QLabel(QStringLiteral("12345"), resultCard);
        pickupCodeLabel->setObjectName("pickupCode");
        copyButton = new QPushButton(text("复制"), resultCard);
        copyButton->setObjectName("lightButton");
        copyButton->setCursor(Qt::PointingHandCursor);
        copyButton->setFixedSize(80, 32);
        codeRow->addWidget(pickupCodeLabel);
        codeRow->addStretch();
        codeRow->addWidget(copyButton);

        auto *expireText = new QLabel(text("文件将在 24 小时后自动过期"), resultCard);
        expireText->setObjectName("hintText");
        resultLayout->addWidget(resultTitle);
        resultLayout->addWidget(resultBody);
        resultLayout->addLayout(codeRow);
        resultLayout->addWidget(expireText);
        resultCard->hide();
        layout->addWidget(resultCard);
        layout->addStretch();

        connect(dropZone, &DropZone::clicked, this, &TransferWindow::chooseFiles);
        connect(dropZone, &DropZone::filesDropped, this, &TransferWindow::setFiles);
        connect(uploadButton, &QPushButton::clicked, this, &TransferWindow::uploadFiles);
        connect(copyButton, &QPushButton::clicked, this, &TransferWindow::copyPickupCode);
        return page;
    }

    QWidget *buildReceivePage() {
        auto *page = new QWidget(this);
        auto *layout = new QVBoxLayout(page);
        layout->setContentsMargins(24, 28, 24, 0);
        layout->setSpacing(16);

        auto *title = new QLabel(text("请输入取件码"), page);
        title->setObjectName("sectionTitle");
        layout->addWidget(title);

        auto *inputLayout = new QHBoxLayout();
        inputLayout->setSpacing(16);
        codeEdit = new QLineEdit(page);
        codeEdit->setObjectName("codeEdit");
        codeEdit->setPlaceholderText(text("请输入 5 位取件码"));
        codeEdit->setMaxLength(5);
        codeEdit->setAlignment(Qt::AlignCenter);
        codeEdit->setFixedHeight(40);

        downloadButton = new QPushButton(text("提取下载"), page);
        downloadButton->setObjectName("primaryButton");
        downloadButton->setCursor(Qt::PointingHandCursor);
        downloadButton->setFixedSize(106, 40);

        inputLayout->addWidget(codeEdit);
        inputLayout->addWidget(downloadButton);
        layout->addLayout(inputLayout);

        downloadProgress = makeProgress(page);
        downloadProgress->hide();
        layout->addWidget(downloadProgress);

        auto *statusCard = new QFrame(page);
        statusCard->setObjectName("statusCard");
        statusCard->setFixedHeight(130);
        auto *statusLayout = new QVBoxLayout(statusCard);
        statusLayout->setContentsMargins(18, 22, 18, 14);
        statusLayout->setSpacing(7);

        auto *statusIcon = new QLabel(QStringLiteral("↓"), statusCard);
        statusIcon->setObjectName("statusIcon");
        statusIcon->setAlignment(Qt::AlignCenter);
        statusTitle = new QLabel(text("等待输入取件码..."), statusCard);
        statusTitle->setObjectName("statusTitle");
        statusTitle->setAlignment(Qt::AlignCenter);
        statusSub = new QLabel(text("输入有效的取件码即可开始下载文件"), statusCard);
        statusSub->setObjectName("hintText");
        statusSub->setAlignment(Qt::AlignCenter);

        statusLayout->addWidget(statusIcon);
        statusLayout->addWidget(statusTitle);
        statusLayout->addWidget(statusSub);
        layout->addWidget(statusCard);

        openFolderCheck = new QCheckBox(text("下载完成后自动打开文件夹"), page);
        unzipCheck = new QCheckBox(text("下载的文件为压缩包则自动解压"), page);
        openFolderCheck->setObjectName("optionCheck");
        unzipCheck->setObjectName("optionCheck");
        openFolderCheck->setChecked(true);
        unzipCheck->setChecked(true);
        layout->addWidget(openFolderCheck);
        layout->addWidget(unzipCheck);
        layout->addStretch();

        connect(downloadButton, &QPushButton::clicked, this, &TransferWindow::downloadByCode);
        connect(codeEdit, &QLineEdit::returnPressed, this, &TransferWindow::downloadByCode);
        connect(codeEdit, &QLineEdit::textChanged, this, &TransferWindow::normalizePickupCode);
        return page;
    }

    QWidget *buildFooter() {
        auto *footer = new QWidget(this);
        footer->setObjectName("footer");
        footer->setFixedHeight(44);
        auto *layout = new QHBoxLayout(footer);
        layout->setContentsMargins(24, 0, 24, 0);
        layout->setSpacing(0);

        startupCheck = new QCheckBox(text("开机自动启动"), footer);
        startupCheck->setObjectName("optionCheck");
        startupCheck->setChecked(isStartupEnabled());
        layout->addWidget(startupCheck);
        layout->addStretch();
        settingsButton = new QPushButton(text("设置中心"), footer);
        settingsButton->setObjectName("lightButton");
        settingsButton->setCursor(Qt::PointingHandCursor);
        settingsButton->setFixedSize(88, 30);
        layout->addWidget(settingsButton);

        connect(startupCheck, &QCheckBox::toggled, this, &TransferWindow::setStartupEnabled);
        connect(settingsButton, &QPushButton::clicked, this, &TransferWindow::openSettingsDialog);
        return footer;
    }

    QProgressBar *makeProgress(QWidget *parent) {
        auto *progress = new QProgressBar(parent);
        progress->setObjectName("thinProgress");
        progress->setTextVisible(false);
        progress->setFixedHeight(6);
        progress->setRange(0, 100);
        progress->setValue(0);
        return progress;
    }

    void applyStyle() {
        QApplication::setStyle(QStringLiteral("Fusion"));
        setStyleSheet(QStringLiteral(R"(
            #mainWindow, QStackedWidget, QStackedWidget > QWidget {
                background: #F9FAFB;
                color: #111827;
                font-family: "Segoe UI", "Microsoft YaHei";
                font-size: 14px;
            }
            QLabel {
                background: transparent;
                color: #111827;
                font-family: "Segoe UI", "Microsoft YaHei";
                font-size: 14px;
            }
            #tabBar {
                background: #F9FAFB;
                border-bottom: 1px solid #E5E7EB;
            }
            #tabButton {
                border: 0;
                background: transparent;
                color: #6B7280;
                font-size: 15px;
                font-weight: 600;
            }
            #tabButton[active="true"] {
                color: #2F7CF6;
                border-bottom: 3px solid #2F7CF6;
            }
            #dropZone {
                background: #FFFFFF;
                border: 2px dashed #5B9DFF;
                border-radius: 12px;
            }
            #dropZone[dragging="true"] {
                background: #F4F8FF;
                border-color: #2F7CF6;
            }
            #dropIcon {
                color: #2F7CF6;
                font-size: 42px;
                font-weight: 700;
            }
            #dropTitle {
                color: #1F2937;
                font-size: 16px;
                font-weight: 600;
            }
            #hintText, #fileHint {
                color: #6B7280;
                font-size: 13px;
            }
            #primaryButton {
                background: #2F7CF6;
                color: #FFFFFF;
                border: 0;
                border-radius: 6px;
                font-size: 16px;
                font-weight: 600;
            }
            #primaryButton:hover {
                background: #1E6EF2;
            }
            #primaryButton:disabled {
                background: #93C5FD;
            }
            #thinProgress {
                border: 0;
                border-radius: 3px;
                background: #E5E7EB;
            }
            #thinProgress::chunk {
                border-radius: 3px;
                background: #2F7CF6;
            }
            #resultCard {
                background: #EFF6FF;
                border: 1px solid #BFDBFE;
                border-radius: 8px;
            }
            #resultTitle {
                color: #1E40AF;
                font-size: 15px;
                font-weight: 600;
            }
            #bodyText {
                color: #374151;
                font-size: 13px;
            }
            #pickupCode {
                color: #2F7CF6;
                font-size: 36px;
                font-weight: 700;
                letter-spacing: 4px;
            }
            #lightButton {
                background: #FFFFFF;
                color: #2F7CF6;
                border: 1px solid #BFDBFE;
                border-radius: 6px;
                font-size: 13px;
            }
            #lightButton:hover {
                background: #F4F8FF;
            }
            #sectionTitle {
                color: #111827;
                font-size: 16px;
                font-weight: 600;
            }
            #codeEdit {
                background: #FFFFFF;
                border: 1px solid #D1D5DB;
                border-radius: 6px;
                color: #374151;
                font-size: 16px;
                padding: 0 12px;
            }
            #codeEdit:focus {
                border: 1px solid #2F7CF6;
            }
            #statusCard {
                background: #FFFFFF;
                border: 1px solid #D1D5DB;
                border-radius: 8px;
            }
            #statusIcon {
                color: #9CA3AF;
                font-size: 42px;
            }
            #statusTitle {
                color: #111827;
                font-size: 16px;
                font-weight: 600;
            }
            #statusTitle[error="true"] {
                color: #EF4444;
            }
            #optionCheck {
                color: #374151;
                font-size: 13px;
                spacing: 10px;
            }
            #optionCheck::indicator {
                width: 16px;
                height: 16px;
            }
            #footer {
                background: #F9FAFB;
                border-top: 1px solid #E5E7EB;
            }
        )"));
    }

    void switchPage(int index) {
        pages->setCurrentIndex(index);
        setActiveTab(sendTab, index == 0);
        setActiveTab(receiveTab, index == 1);
    }

    void setActiveTab(QPushButton *button, bool active) {
        button->setProperty("active", active);
        button->style()->unpolish(button);
        button->style()->polish(button);
        button->update();
    }

    void chooseFiles() {
        const QStringList files = QFileDialog::getOpenFileNames(this, text("选择文件"));
        if (!files.isEmpty()) {
            setFiles(files);
        }
    }

    void setFiles(const QStringList &filePaths) {
        selectedFiles = filePaths;
        dropZone->setFiles(filePaths);
        resultCard->hide();
        appendLog(QStringLiteral("selected files: %1").arg(filePaths.size()));
    }

    void uploadFiles() {
        if (!ensureServerConfigured()) {
            return;
        }
        if (selectedFiles.isEmpty()) {
            chooseFiles();
        }
        if (selectedFiles.isEmpty()) {
            return;
        }

        auto *multipart = new QHttpMultiPart(QHttpMultiPart::FormDataType);
        for (const QString &filePath : selectedFiles) {
            auto *file = new QFile(filePath);
            if (!file->open(QIODevice::ReadOnly)) {
                appendLog(QStringLiteral("failed to open upload file: %1").arg(filePath));
                file->deleteLater();
                continue;
            }

            QHttpPart filePart;
            filePart.setHeader(
                QNetworkRequest::ContentDispositionHeader,
                QStringLiteral("form-data; name=\"file\"; filename=\"%1\"").arg(QFileInfo(filePath).fileName())
            );
            filePart.setBodyDevice(file);
            file->setParent(multipart);
            multipart->append(filePart);
        }

        QNetworkRequest request(QUrl(serverBaseUrl() + QStringLiteral("/upload")));
        QNetworkReply *reply = networkManager->post(request, multipart);
        multipart->setParent(reply);

        uploadButton->setEnabled(false);
        uploadProgress->setValue(0);
        uploadProgress->show();
        appendLog("upload started");

        connect(reply, &QNetworkReply::uploadProgress, this, [this](qint64 sent, qint64 total) {
            if (total > 0) {
                uploadProgress->setValue(static_cast<int>(sent * 100 / total));
            }
        });

        connect(reply, &QNetworkReply::finished, this, [this, reply]() {
            uploadButton->setEnabled(true);
            uploadProgress->hide();

            if (reply->error() == QNetworkReply::NoError) {
                lastPickupCode = QString::fromUtf8(reply->readAll()).trimmed();
                pickupCodeLabel->setText(lastPickupCode);
                resultCard->show();
                appendLog(QStringLiteral("upload success: %1").arg(lastPickupCode));
            } else {
                appendLog(QStringLiteral("upload failed: %1").arg(reply->errorString()));
                QMessageBox::warning(this, text("上传失败"), reply->errorString());
            }

            reply->deleteLater();
        });
    }

    void copyPickupCode() {
        if (lastPickupCode.isEmpty()) {
            return;
        }
        QApplication::clipboard()->setText(lastPickupCode);
        copyButton->setText(text("已复制"));
        QTimer::singleShot(1200, this, [this]() {
            copyButton->setText(text("复制"));
        });
    }

    void normalizePickupCode(const QString &value) {
        QString digits;
        for (const QChar character : value) {
            if (character.isDigit() && digits.size() < 5) {
                digits.append(character);
            }
        }
        if (digits != value) {
            const QSignalBlocker blocker(codeEdit);
            codeEdit->setText(digits);
        }
    }

    void downloadByCode() {
        if (!ensureServerConfigured()) {
            return;
        }
        const QString code = codeEdit->text().trimmed();
        if (code.size() != 5) {
            setStatus(text("取件码格式错误"), text("请输入 5 位数字取件码"), true);
            return;
        }

        downloadButton->setEnabled(false);
        setStatus(text("查询文件..."), text("正在连接服务器"), false);
        appendLog(QStringLiteral("head download: %1").arg(code));

        const QUrl url(serverBaseUrl() + QStringLiteral("/download/") + code);
        QNetworkReply *headReply = networkManager->head(QNetworkRequest(url));
        connect(headReply, &QNetworkReply::finished, this, [this, headReply, url, code]() {
            if (headReply->error() != QNetworkReply::NoError) {
                downloadButton->setEnabled(true);
                setStatus(text("取件码不存在"), text("请检查取件码是否正确"), true);
                appendLog(QStringLiteral("head failed: %1").arg(headReply->errorString()));
                headReply->deleteLater();
                return;
            }

            const QString fileName = fileNameFromReply(headReply, code);
            headReply->deleteLater();
            startDownload(url, fileName);
        });
    }

    void startDownload(const QUrl &url, const QString &fileName) {
        QString downloadDir = QStandardPaths::writableLocation(QStandardPaths::DownloadLocation);
        if (downloadDir.isEmpty()) {
            downloadDir = QDir::homePath();
        }
        activeDownloadPath = uniquePath(downloadDir, fileName);
        activeDownloadFile = new QFile(activeDownloadPath, this);

        if (!activeDownloadFile->open(QIODevice::WriteOnly)) {
            downloadButton->setEnabled(true);
            setStatus(text("无法写入文件"), activeDownloadPath, true);
            appendLog(QStringLiteral("open download file failed: %1").arg(activeDownloadPath));
            activeDownloadFile->deleteLater();
            activeDownloadFile = nullptr;
            return;
        }

        QNetworkReply *reply = networkManager->get(QNetworkRequest(url));
        downloadProgress->setValue(0);
        downloadProgress->show();
        setStatus(text("正在下载..."), QFileInfo(activeDownloadPath).fileName(), false);
        appendLog(QStringLiteral("download started: %1").arg(activeDownloadPath));

        connect(reply, &QNetworkReply::readyRead, this, [this, reply]() {
            if (activeDownloadFile) {
                activeDownloadFile->write(reply->readAll());
            }
        });

        connect(reply, &QNetworkReply::downloadProgress, this, [this](qint64 received, qint64 total) {
            if (total > 0) {
                downloadProgress->setValue(static_cast<int>(received * 100 / total));
            }
        });

        connect(reply, &QNetworkReply::finished, this, [this, reply]() {
            if (activeDownloadFile) {
                activeDownloadFile->write(reply->readAll());
                activeDownloadFile->flush();
                activeDownloadFile->close();
            }

            downloadButton->setEnabled(true);
            downloadProgress->hide();

            if (reply->error() == QNetworkReply::NoError) {
                QString finalPath = activeDownloadPath;
                if (unzipCheck->isChecked() && QFileInfo(activeDownloadPath).suffix().compare("zip", Qt::CaseInsensitive) == 0) {
                    finalPath = unzipWithShell(activeDownloadPath);
                }
                setStatus(text("下载完成"), finalPath, false);
                appendLog(QStringLiteral("download success: %1").arg(finalPath));
                if (openFolderCheck->isChecked()) {
                    QDesktopServices::openUrl(QUrl::fromLocalFile(QFileInfo(finalPath).absolutePath()));
                }
            } else {
                setStatus(text("下载失败"), reply->errorString(), true);
                appendLog(QStringLiteral("download failed: %1").arg(reply->errorString()));
            }

            if (activeDownloadFile) {
                activeDownloadFile->deleteLater();
                activeDownloadFile = nullptr;
            }
            reply->deleteLater();
        });
    }

    QString unzipWithShell(const QString &zipPath) {
        const QFileInfo zipInfo(zipPath);
        const QString targetDir = QDir(zipInfo.absolutePath()).filePath(zipInfo.completeBaseName());
        QDir().mkpath(targetDir);
        QString escapedZipPath = zipPath;
        QString escapedTargetDir = targetDir;
        escapedZipPath.replace("'", "''");
        escapedTargetDir.replace("'", "''");

        const QString script =
            QStringLiteral("$shell=New-Object -ComObject Shell.Application;")
            + QStringLiteral("$zip=$shell.NameSpace('%1');").arg(escapedZipPath)
            + QStringLiteral("$dst=$shell.NameSpace('%1');").arg(escapedTargetDir)
            + QStringLiteral("if($zip -and $dst){$dst.CopyHere($zip.Items(),16);Start-Sleep -Milliseconds 500}");

        const int exitCode = QProcess::execute(QStringLiteral("powershell.exe"), QStringList()
            << QStringLiteral("-NoProfile")
            << QStringLiteral("-ExecutionPolicy")
            << QStringLiteral("Bypass")
            << QStringLiteral("-Command")
            << script);

        if (exitCode == 0) {
            appendLog(QStringLiteral("unzip success: %1").arg(targetDir));
            return targetDir;
        }

        appendLog(QStringLiteral("unzip failed, exit: %1").arg(exitCode));
        return zipPath;
    }

    void setStatus(const QString &title, const QString &sub, bool error) {
        statusTitle->setText(title);
        statusSub->setText(sub);
        statusTitle->setProperty("error", error);
        statusTitle->style()->unpolish(statusTitle);
        statusTitle->style()->polish(statusTitle);
        statusTitle->update();
    }

    bool isStartupEnabled() const {
        QSettings settings(QStringLiteral("HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Run"), QSettings::NativeFormat);
        return settings.contains(QStringLiteral("zFileTransferStationQt"));
    }

    void setStartupEnabled(bool enabled) {
        QSettings settings(QStringLiteral("HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Run"), QSettings::NativeFormat);
        if (enabled) {
            const QString exePath = QDir::toNativeSeparators(QCoreApplication::applicationFilePath());
            settings.setValue(QStringLiteral("zFileTransferStationQt"), QStringLiteral("\"%1\"").arg(exePath));
            appendLog("startup enabled");
        } else {
            settings.remove(QStringLiteral("zFileTransferStationQt"));
            appendLog("startup disabled");
        }
    }

    QString serverBaseUrl() const {
        QSettings settings;
        settings.beginGroup(QString::fromLatin1(kConfigGroup));
        QString value = settings.value(QString::fromLatin1(kServerConfigKey)).toString();
        settings.endGroup();
        value = value.trimmed();
        while (value.endsWith('/')) {
            value.chop(1);
        }
        if (!value.startsWith(QStringLiteral("http://"), Qt::CaseInsensitive)
            && !value.startsWith(QStringLiteral("https://"), Qt::CaseInsensitive)) {
            value.prepend(QStringLiteral("http://"));
        }
        return value;
    }

    void saveServerBaseUrl(const QString &url) {
        QSettings settings;
        settings.beginGroup(QString::fromLatin1(kConfigGroup));
        settings.setValue(QString::fromLatin1(kServerConfigKey), url);
        settings.endGroup();
        appendLog(QStringLiteral("server url changed: %1").arg(url));
    }

    void openSettingsDialog() {
        showServerSettingsDialog(false);
    }

    bool ensureServerConfigured() {
        if (!serverBaseUrl().isEmpty()) {
            return true;
        }
        return showServerSettingsDialog(true);
    }

    QString normalizeServerInput(QString url) const {
        url = url.trimmed();
        while (url.endsWith('/')) {
            url.chop(1);
        }
        if (!url.isEmpty()
            && !url.startsWith(QStringLiteral("http://"), Qt::CaseInsensitive)
            && !url.startsWith(QStringLiteral("https://"), Qt::CaseInsensitive)) {
            url.prepend(QStringLiteral("http://"));
        }
        return url;
    }

    bool showServerSettingsDialog(bool required) {
        QDialog dialog(this);
        dialog.setWindowTitle(required ? text("首次设置服务端") : text("设置中心"));
        dialog.setModal(true);
        dialog.setFixedWidth(390);

        auto *layout = new QVBoxLayout(&dialog);
        layout->setContentsMargins(18, 18, 18, 18);
        layout->setSpacing(12);

        auto *title = new QLabel(required ? text("请先设置服务端地址") : text("服务端地址"), &dialog);
        title->setObjectName("sectionTitle");
        auto *desc = new QLabel(text("例如：http://服务器IP:8080 或 https://你的域名"), &dialog);
        desc->setObjectName("hintText");
        desc->setWordWrap(true);
        auto *edit = new QLineEdit(&dialog);
        edit->setObjectName("codeEdit");
        edit->setPlaceholderText(text("http://服务器IP:8080"));
        edit->setText(serverBaseUrl());
        edit->setFixedHeight(40);
        auto *status = new QLabel(required ? text("测试连接通过后才能保存。") : text("修改后请先测试连接。"), &dialog);
        status->setObjectName("hintText");
        status->setWordWrap(true);

        auto *buttons = new QHBoxLayout();
        auto *cancelButton = new QPushButton(text("取消"), &dialog);
        cancelButton->setObjectName("lightButton");
        auto *testButton = new QPushButton(text("测试连接"), &dialog);
        testButton->setObjectName("lightButton");
        auto *saveButton = new QPushButton(text("保存生效"), &dialog);
        saveButton->setObjectName("primaryButton");
        saveButton->setFixedHeight(34);
        saveButton->setEnabled(false);
        buttons->addStretch();
        buttons->addWidget(cancelButton);
        buttons->addWidget(testButton);
        buttons->addWidget(saveButton);

        layout->addWidget(title);
        layout->addWidget(desc);
        layout->addWidget(edit);
        layout->addWidget(status);
        layout->addLayout(buttons);

        QString testedUrl;
        connect(edit, &QLineEdit::textChanged, &dialog, [&]() {
            testedUrl.clear();
            saveButton->setEnabled(false);
            status->setText(text("修改后请先测试连接。"));
        });
        connect(cancelButton, &QPushButton::clicked, &dialog, [&]() {
            dialog.reject();
        });
        connect(testButton, &QPushButton::clicked, &dialog, [&]() {
            const QString url = normalizeServerInput(edit->text());
            if (url.isEmpty()) {
                status->setText(text("服务端地址不能为空。"));
                return;
            }
            testButton->setEnabled(false);
            saveButton->setEnabled(false);
            status->setText(text("正在测试连接..."));

            QNetworkAccessManager manager;
            QNetworkRequest request(QUrl(url + QStringLiteral("/health")));
            QNetworkReply *reply = manager.get(request);
            QEventLoop loop;
            QTimer timer;
            timer.setSingleShot(true);
            connect(reply, &QNetworkReply::finished, &loop, &QEventLoop::quit);
            connect(&timer, &QTimer::timeout, &loop, &QEventLoop::quit);
            timer.start(8000);
            loop.exec();

            bool ok = false;
            QString errorText;
            if (timer.isActive() && reply->error() == QNetworkReply::NoError) {
                const QString body = QString::fromUtf8(reply->readAll()).trimmed().toLower();
                ok = body == QStringLiteral("ok");
                if (!ok) {
                    errorText = text("健康检查返回异常。");
                }
            } else if (!timer.isActive()) {
                errorText = text("连接超时，请检查地址和端口。");
            } else {
                errorText = reply->errorString();
            }
            reply->deleteLater();
            testButton->setEnabled(true);

            if (ok) {
                testedUrl = url;
                saveButton->setEnabled(true);
                status->setText(text("连接成功，可以保存：") + url);
            } else {
                testedUrl.clear();
                saveButton->setEnabled(false);
                status->setText(text("连接失败：") + errorText);
            }
        });
        connect(saveButton, &QPushButton::clicked, &dialog, [&]() {
            if (testedUrl.isEmpty()) {
                return;
            }
            saveServerBaseUrl(testedUrl);
            dialog.accept();
        });

        const bool accepted = dialog.exec() == QDialog::Accepted;
        if (!accepted && required) {
            setStatus(text("未设置服务端"), text("请点击设置中心，测试连接通过后保存服务端地址。"), true);
        }
        return accepted || !serverBaseUrl().isEmpty();
    }
};

int main(int argc, char *argv[]) {
    QCoreApplication::setAttribute(Qt::AA_EnableHighDpiScaling);
    QCoreApplication::setAttribute(Qt::AA_UseHighDpiPixmaps);

    QApplication app(argc, argv);
    QApplication::setApplicationName(text("文件中转站"));
    QApplication::setOrganizationName(QStringLiteral("zFileTransferStation"));

    TransferWindow window;
    window.show();
    return app.exec();
}

#include "main.moc"
