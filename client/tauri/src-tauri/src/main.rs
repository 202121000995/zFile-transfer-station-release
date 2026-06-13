#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use serde::Serialize;
use std::fs::{self, File};
use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::process::Command;
use std::time::Duration;

const STARTUP_NAME: &str = "文件中转站";

#[derive(Clone, Serialize)]
struct DownloadProgress {
    percent: u8,
    title: String,
    message: String,
}

fn emit_progress(window: &tauri::Window, percent: u8, title: &str, message: &str) {
    let _ = window.emit(
        "download-progress",
        DownloadProgress {
            percent,
            title: title.to_string(),
            message: message.to_string(),
        },
    );
}

fn downloads_dir() -> Result<PathBuf, String> {
    let profile = std::env::var("USERPROFILE").map_err(|_| "无法读取用户目录".to_string())?;
    let dir = PathBuf::from(profile).join("Downloads");
    fs::create_dir_all(&dir).map_err(|e| format!("无法创建下载目录：{e}"))?;
    Ok(dir)
}

fn sanitize_file_name(name: &str) -> String {
    let cleaned: String = name
        .chars()
        .map(|ch| match ch {
            '<' | '>' | ':' | '"' | '/' | '\\' | '|' | '?' | '*' | '\0' => '_',
            _ => ch,
        })
        .collect();
    let trimmed = cleaned.trim().trim_matches('.').to_string();
    if trimmed.is_empty() {
        "download.bin".to_string()
    } else {
        trimmed
    }
}

fn unique_path(dir: &Path, file_name: &str) -> PathBuf {
    let base = Path::new(file_name)
        .file_stem()
        .and_then(|s| s.to_str())
        .unwrap_or("download");
    let ext = Path::new(file_name)
        .extension()
        .and_then(|s| s.to_str())
        .map(|s| format!(".{s}"))
        .unwrap_or_default();

    let mut candidate = dir.join(file_name);
    let mut index = 1;
    while candidate.exists() {
        candidate = dir.join(format!("{base} ({index}){ext}"));
        index += 1;
    }
    candidate
}

fn unzip_file(zip_path: &Path) -> Result<PathBuf, String> {
    let file = File::open(zip_path).map_err(|e| format!("无法打开压缩包：{e}"))?;
    let mut archive = zip::ZipArchive::new(file).map_err(|e| format!("无法读取压缩包：{e}"))?;
    let target_dir = zip_path.with_extension("");
    fs::create_dir_all(&target_dir).map_err(|e| format!("无法创建解压目录：{e}"))?;

    for index in 0..archive.len() {
        let mut entry = archive
            .by_index(index)
            .map_err(|e| format!("无法读取压缩包文件：{e}"))?;
        let Some(enclosed_name) = entry.enclosed_name().map(|p| p.to_path_buf()) else {
            continue;
        };
        let out_path = target_dir.join(enclosed_name);
        if entry.name().ends_with('/') {
            fs::create_dir_all(&out_path).map_err(|e| format!("无法创建目录：{e}"))?;
            continue;
        }
        if let Some(parent) = out_path.parent() {
            fs::create_dir_all(parent).map_err(|e| format!("无法创建目录：{e}"))?;
        }
        let mut out_file = File::create(&out_path).map_err(|e| format!("无法写入解压文件：{e}"))?;
        std::io::copy(&mut entry, &mut out_file).map_err(|e| format!("解压失败：{e}"))?;
    }

    Ok(target_dir)
}

fn open_in_explorer(path: &Path) {
    let _ = Command::new("explorer.exe")
        .arg(format!("/select,{}", path.display()))
        .spawn();
}

fn normalize_server(server: &str) -> Result<String, String> {
    let mut value = server.trim().trim_end_matches('/').to_string();
    if value.is_empty() {
        return Err("请先设置服务端地址".to_string());
    }
    if !value.starts_with("http://") && !value.starts_with("https://") {
        value = format!("http://{value}");
    }
    Ok(value)
}

#[tauri::command]
fn download_file(window: tauri::Window, server: String, code: String, auto_open: bool, auto_unzip: bool) -> Result<String, String> {
    if code.len() != 5 || !code.chars().all(|ch| ch.is_ascii_digit()) {
        return Err("请输入 5 位数字取件码".to_string());
    }
    let server = normalize_server(&server)?;

    let agent = ureq::AgentBuilder::new()
        .timeout_connect(Duration::from_secs(15))
        .timeout_read(Duration::from_secs(300))
        .timeout_write(Duration::from_secs(300))
        .build();

    let url = format!("{server}/download/{code}");
    emit_progress(&window, 5, "查询文件...", "正在连接服务器");

    let head = agent
        .head(&url)
        .call()
        .map_err(|_| "取件码不存在或服务器不可达".to_string())?;

    let raw_name = head
        .header("X-File-Name")
        .or_else(|| head.header("Content-Disposition"))
        .unwrap_or(&code);
    let file_name = if raw_name.contains("filename=") {
        raw_name
            .split("filename=")
            .nth(1)
            .unwrap_or(raw_name)
            .trim_matches('"')
            .to_string()
    } else {
        raw_name.to_string()
    };
    let file_name = sanitize_file_name(&file_name);
    let total = head
        .header("Content-Length")
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(0);

    let dir = downloads_dir()?;
    let path = unique_path(&dir, &file_name);
    emit_progress(&window, 8, "开始下载...", &file_name);

    let response = agent
        .get(&url)
        .call()
        .map_err(|_| "下载失败：取件码不存在或服务器不可达".to_string())?;

    let mut reader = response.into_reader();
    let mut file = File::create(&path).map_err(|e| format!("无法创建文件：{e}"))?;
    let mut buffer = [0_u8; 64 * 1024];
    let mut downloaded = 0_u64;

    loop {
        let read = reader.read(&mut buffer).map_err(|e| format!("读取网络数据失败：{e}"))?;
        if read == 0 {
            break;
        }
        file.write_all(&buffer[..read])
            .map_err(|e| format!("写入文件失败：{e}"))?;
        downloaded += read as u64;
        if total > 0 {
            let percent = ((downloaded * 90 / total) + 8).min(98) as u8;
            emit_progress(&window, percent, "正在下载...", &format!("{} / {} 字节", downloaded, total));
        }
    }
    file.flush().map_err(|e| format!("保存文件失败：{e}"))?;

    let mut final_path = path.clone();
    if auto_unzip
        && path
            .extension()
            .and_then(|ext| ext.to_str())
            .map(|ext| ext.eq_ignore_ascii_case("zip"))
            .unwrap_or(false)
    {
        emit_progress(&window, 99, "正在解压...", &file_name);
        match unzip_file(&path) {
            Ok(unzip_dir) => final_path = unzip_dir,
            Err(err) => emit_progress(&window, 100, "下载完成，解压失败", &err),
        }
    }

    emit_progress(&window, 100, "下载完成", &final_path.display().to_string());
    if auto_open {
        open_in_explorer(&final_path);
    }

    Ok(final_path.display().to_string())
}

#[tauri::command]
fn get_startup() -> bool {
    let output = Command::new("reg")
        .args([
            "query",
            r"HKCU\Software\Microsoft\Windows\CurrentVersion\Run",
            "/v",
            STARTUP_NAME,
        ])
        .output();
    output.map(|out| out.status.success()).unwrap_or(false)
}

#[tauri::command]
fn set_startup(enabled: bool) -> Result<(), String> {
    let exe = std::env::current_exe().map_err(|e| format!("无法读取程序路径：{e}"))?;
    let exe = format!("\"{}\"", exe.display());

    let status = if enabled {
        Command::new("reg")
            .args([
                "add",
                r"HKCU\Software\Microsoft\Windows\CurrentVersion\Run",
                "/v",
                STARTUP_NAME,
                "/t",
                "REG_SZ",
                "/d",
                &exe,
                "/f",
            ])
            .status()
    } else {
        Command::new("reg")
            .args([
                "delete",
                r"HKCU\Software\Microsoft\Windows\CurrentVersion\Run",
                "/v",
                STARTUP_NAME,
                "/f",
            ])
            .status()
    };

    match status {
        Ok(code) if code.success() => Ok(()),
        Ok(_) if !enabled => Ok(()),
        Ok(_) => Err("注册表写入失败".to_string()),
        Err(e) => Err(format!("无法调用 reg.exe：{e}")),
    }
}

fn main() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![
            download_file,
            get_startup,
            set_startup
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
