param(
    [ValidateSet("x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc")]
    [string]$Target = "x86_64-pc-windows-msvc"
)

$ErrorActionPreference = "Stop"
$FrontendDir = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot "../../frontend"))
Push-Location $FrontendDir
try {
    # Requires Node 24+, pnpm, Go, Rust and Visual Studio C++ build tools.
    # The Windows config is merged automatically by Tauri for this target.
    & pnpm tauri:build -- --target $Target
    if ($LASTEXITCODE -ne 0) { throw "Windows Preview build failed ($LASTEXITCODE)" }
    $BundleDir = Join-Path $FrontendDir "../tauri/target/$Target/release/bundle/nsis"
    Get-ChildItem $BundleDir -Filter "*-setup.exe" | ForEach-Object {
        Write-Host $_.FullName
        Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName
    }
} finally {
    Pop-Location
}
