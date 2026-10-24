fn main() {
    println!("cargo:rerun-if-changed=../upstream.lock.json");
    println!("cargo:rerun-if-changed=../runtime-tools.lock.json");
    println!("cargo:rerun-if-changed=../guest-packages.lock.json");
    tauri_build::build()
}
