#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod backend_paths;
mod client_config;
mod client_job;
mod client_recovery;
mod client_worker;
#[cfg(windows)]
mod credential_writer;
mod desktop;
mod desktop_policy;
mod engine;
mod engine_proxy;
mod host_capacity;
mod installation;
mod local_link;
mod local_services;
mod migration_mapping;
mod mounts;
mod native_client;
mod native_files;
mod network_diagnostics;
mod operation_worker;
mod operations;
mod private_setup_client;
mod persistent_setup_model;
mod persistent_security_policy;
mod process;
#[cfg(all(test, windows))]
mod process_fixture;
mod readiness;
mod registry;
mod runtime;
mod runtime_winio_provenance;
mod runtime_cache;
mod runtime_inspection;
mod runtime_inspection_transport;
mod runtime_inspection_transport_model;
mod runtime_inspection_transport_io;
mod sdk_identity;
mod setup_logs;
#[path = "session.rs"]
mod terminal;
mod wsl;
mod wsl_clients;
mod wsl_integration;
mod wsl_tools;
use serde::Serialize;
use std::path::PathBuf;

// rdd remains the lifecycle authority. No window event handler stops it, and no
// read command starts it. Only the verified owned endpoint is read; lifecycle
// lifecycle changes are explicit requests forwarded to the owned rdd instance.
#[tauri::command]
async fn backend_status() -> engine::Status {
    engine::status().await
}

#[tauri::command]
async fn engine_inventory() -> Result<engine::Inventory, String> {
    engine::inventory().await
}
#[tauri::command]
async fn engine_disk_info() -> Result<engine::DiskInfo, String> {
    engine::disk_info().await
}
#[tauri::command]
async fn engine_proxy_capability() -> Result<bool, String> {
    tokio::task::spawn_blocking(engine_proxy::capability)
        .await
        .map_err(|_| "Backend capability check did not complete.".to_string())?
}
#[tauri::command]
async fn save_migration_mapping(
    contents: String,
    target: String,
) -> Result<Option<migration_mapping::SavedMapping>, String> {
    migration_mapping::save(contents, target).await
}

#[tauri::command]
async fn inspect_migration_mapping(
    path: String,
    expected_sha: String,
    contents: String,
    target: String,
) -> Result<(), String> {
    migration_mapping::inspect(path, expected_sha, contents, target).await
}

#[tauri::command]
async fn local_services_request(
    request: local_services::Request,
) -> Result<serde_json::Value, String> {
    local_services::invoke(request).await
}

#[tauri::command]
async fn engine_run(
    state: tauri::State<'_, operations::Operations>,
    id: String,
    request: operations::Request,
    output: tauri::ipc::Channel<operations::Output>,
) -> Result<operations::Outcome, String> {
    operations::run(&state, id, request, output).await
}

#[tauri::command]
async fn terminal_open(
    state: tauri::State<'_, terminal::Sessions>,
    id: String,
    container: String,
    shell: terminal::Shell,
    cols: u16,
    rows: u16,
    output: tauri::ipc::Channel<terminal::Event>,
) -> Result<(), String> {
    terminal::open(&state, id, container, shell, cols, rows, output).await
}
#[tauri::command]
async fn terminal_open_pod(
    state: tauri::State<'_, terminal::Sessions>,
    id: String,
    pod: terminal::PodTarget,
    shell: terminal::Shell,
    cols: u16,
    rows: u16,
    output: tauri::ipc::Channel<terminal::Event>,
) -> Result<(), String> {
    terminal::open_pod(&state, id, pod, shell, cols, rows, output).await
}
#[tauri::command]
async fn terminal_input(
    state: tauri::State<'_, terminal::Sessions>,
    id: String,
    bytes: Vec<u8>,
) -> Result<(), String> {
    terminal::input(&state, &id, bytes).await
}
#[tauri::command]
fn terminal_close(state: tauri::State<'_, terminal::Sessions>, id: String) -> Result<(), String> {
    terminal::close(&state, &id)
}
#[tauri::command]
fn terminal_ack(
    state: tauri::State<'_, terminal::Sessions>,
    id: String,
    bytes: usize,
) -> Result<(), String> {
    terminal::acknowledge(&state, &id, bytes)
}
#[tauri::command]
async fn terminal_resize(
    state: tauri::State<'_, terminal::Sessions>,
    id: String,
    cols: u16,
    rows: u16,
) -> Result<(), String> {
    terminal::resize(&state, &id, cols, rows).await
}
#[tauri::command]
async fn choose_path(
    window: tauri::WebviewWindow,
    directory: bool,
) -> Result<Option<String>, String> {
    let dialog = rfd::AsyncFileDialog::new().set_parent(&window);
    let selected = if directory {
        dialog.pick_folder().await
    } else {
        dialog.pick_file().await
    };
    selected
        .map(|file| {
            file.path()
                .to_str()
                .map(String::from)
                .ok_or("Selected path is not valid Unicode.".into())
        })
        .transpose()
}
#[tauri::command]
async fn export_text(window: tauri::WebviewWindow, contents: String) -> Result<bool, String> {
    if contents.len() > 2 * 1024 * 1024 {
        return Err("Export exceeds the 2 MiB limit.".into());
    }
    let file = rfd::AsyncFileDialog::new()
        .set_parent(&window)
        .set_file_name("orbwin-output.log")
        .save_file()
        .await;
    if let Some(file) = file {
        file.write(contents.as_bytes())
            .await
            .map_err(|e| e.to_string())?;
        Ok(true)
    } else {
        Ok(false)
    }
}
#[tauri::command]
async fn choose_save_path(window: tauri::WebviewWindow) -> Result<Option<String>, String> {
    rfd::AsyncFileDialog::new()
        .set_parent(&window)
        .set_file_name("orbwin-file.bin")
        .save_file()
        .await
        .map(|file| {
            file.path()
                .to_str()
                .map(String::from)
                .ok_or("Selected path is not valid Unicode.".into())
        })
        .transpose()
}

#[tauri::command]
fn engine_cancel(
    state: tauri::State<'_, operations::Operations>,
    id: String,
) -> Result<(), String> {
    operations::cancel(&state, &id)
}

#[tauri::command]
fn engine_setup_decide(
    state: tauri::State<'_, operations::Operations>,
    id: String,
    nonce: String,
) -> Result<(), String> {
    operations::setup_decide(&state, &id, &nonce)
}

#[derive(Serialize)]
struct Diagnostic {
    name: &'static str,
    output: String,
    success: bool,
}

#[tauri::command]
async fn export_support(window: tauri::WebviewWindow) -> Result<bool, String> {
    let snapshot = serde_json::json!({ "schema":1, "product":"OrbWin", "version":env!("CARGO_PKG_VERSION"), "backendSource":serde_json::from_str::<serde_json::Value>(include_str!("../../upstream.lock.json")).map_err(|e| e.to_string())?["commit"], "connection":engine::status().await, "host":host_diagnostics().await });
    let file = rfd::AsyncFileDialog::new()
        .set_parent(&window)
        .set_file_name("orbwin-diagnostics.json")
        .save_file()
        .await;
    if let Some(file) = file {
        file.write(&serde_json::to_vec_pretty(&snapshot).map_err(|e| e.to_string())?)
            .await
            .map_err(|e| e.to_string())?;
        Ok(true)
    } else {
        Ok(false)
    }
}

#[tauri::command]
async fn host_diagnostics() -> Vec<Diagnostic> {
    // Fixed binaries + fixed read-only arguments. No renderer-controlled paths,
    // shell commands, environment dumps, context switches or startup commands.
    let system_root = std::env::var_os("SystemRoot").unwrap_or_else(|| "C:\\Windows".into());
    let wsl = PathBuf::from(system_root).join("System32/wsl.exe");
    let mut results = Vec::new();
    for (name, args) in [
        ("WSL version", vec!["--version"]),
        ("WSL distributions", vec!["--list", "--verbose"]),
    ] {
        let result = process::read_command(&wsl, &args).await;
        results.push(match result {
            Ok(output) => Diagnostic {
                name,
                output,
                success: true,
            },
            Err(error) => Diagnostic {
                name,
                output: error,
                success: false,
            },
        });
    }
    results
}

fn main() {
    if let Some(status) = desktop::early_maintenance() {
        std::process::exit(status);
    }
    tauri::Builder::default()
        .manage(desktop::Desktop::default())
        .setup(desktop::setup)
        .manage(operations::Operations::default())
        .manage(terminal::Sessions::default())
        .manage(client_recovery::Reviews::default())
        .invoke_handler(tauri::generate_handler![
            setup_logs::open_setup_logs,
            client_recovery::client_recovery_review,
            client_recovery::client_bootstrap_review,
            client_recovery::client_recovery_cancel,
            client_recovery::client_recovery_restore,
            desktop::desktop_preferences,
            desktop::desktop_set_startup,
            backend_status,
            engine_inventory,
            engine_disk_info,
            save_migration_mapping,
            inspect_migration_mapping,
            local_services_request,
            engine_proxy_capability,
            engine_run,
            engine_cancel,
            engine_setup_decide,
            terminal_open,
            terminal_open_pod,
            terminal_input,
            terminal_close,
            terminal_ack,
            terminal_resize,
            choose_path,
            choose_save_path,
            native_files::open_selected_copy,
            native_files::open_local_copy,
            export_text,
            export_support,
            network_diagnostics::network_diagnostics,
            host_diagnostics,
            local_link::open_local_port,
            wsl::wsl_open_terminal,
            wsl_tools::open_wsl_project,
            wsl_clients::wsl_refresh_private_clients,
            wsl_integration::wsl_project,
            wsl_integration::wsl_migration_project
        ])
        .run(tauri::generate_context!())
        .expect("OrbWin desktop initialization failed");
}
