# QMediaSync FPK Pack PowerShell Script

function Write-Colored($Color, $Message) {
    switch ($Color) {
        "green" { Write-Host $Message -ForegroundColor Green }
        "red" { Write-Host $Message -ForegroundColor Red }
        "yellow" { Write-Host $Message -ForegroundColor Yellow }
        "cyan" { Write-Host $Message -ForegroundColor Cyan }
        default { Write-Host $Message }
    }
}

Write-Colored "cyan" "切换到项目根目录..."
Set-Location -Path ".."
Write-Colored "green" "已切换工作目录：$(Get-Location)"

Write-Colored "green" "========================================"
Write-Colored "green" "QMediaSync FPK Pack Script"
Write-Colored "green" "========================================"

if (!(Test-Path -Path ".git" -PathType Container)) {
    Write-Colored "red" "Error: Not a Git repository"
    exit 1
}

Write-Colored "cyan" "确定版本标签..."
try {
    $TAG = git describe --tags --exact-match 2>$null
    if ([string]::IsNullOrEmpty($TAG)) {
        Write-Colored "yellow" "Warning: No exact Git tag found, using latest tag"
        $TAG = git describe --tags 2>$null
        if ([string]::IsNullOrEmpty($TAG)) {
            Write-Colored "red" "Error: No Git tag found"
            exit 1
        }
    }
    Write-Colored "cyan" "使用标签: $TAG"
} catch {
    Write-Colored "red" "Error: Failed to get Git tag"
    exit 1
}

Write-Colored "green" "========================================"
Write-Colored "green" "开始构建可执行文件..."
Write-Colored "green" "========================================"

if (Test-Path -Path "temp_build" -PathType Container) {
    Write-Colored "yellow" "清理现有temp_build目录..."
    Remove-Item -Path "temp_build" -Recurse -Force
}
New-Item -Path "temp_build" -ItemType Directory -Force | Out-Null

Write-Colored "cyan" "安装所有项目依赖..."
go mod tidy

Write-Colored "cyan" "编译Linux x86_64 (amd64)架构可执行文件..."
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

$PUBLISH_DATE = Get-Date -Format "yyyy-MM-dd HH:mm:ss"

$ENCRYPTION_KEY = $env:ENCRYPTION_KEY

$EXE_NAME = "QMediaSync"
$LDFLAGS = "-s -w -X main.Version=$TAG -X 'main.PublishDate=$PUBLISH_DATE' -X main.ENCRYPTION_KEY=$ENCRYPTION_KEY"

Write-Colored "cyan" "执行编译命令..."
try {
    go build -ldflags "$LDFLAGS" -o "temp_build/$EXE_NAME"
    if ($LASTEXITCODE -ne 0) {
        throw "Build failed"
    }
    Write-Colored "green" "编译完成"

    Rename-Item -Path "temp_build/$EXE_NAME" -NewName "QMediaSync_linux_amd64_exe" -Force
    Write-Colored "green" "重命名可执行文件为 QMediaSync_linux_amd64_exe"
} catch {
    Write-Colored "red" "Error: Build failed for linux/amd64"
    Write-Colored "red" $_.Exception.Message
    exit 1
}

Write-Colored "cyan" "编译Linux arm64架构可执行文件..."
$env:GOOS = "linux"
$env:GOARCH = "arm64"
$env:CGO_ENABLED = "0"

try {
    go build -ldflags "$LDFLAGS" -o "temp_build/$EXE_NAME"
    if ($LASTEXITCODE -ne 0) {
        throw "Build failed"
    }
    Write-Colored "green" "编译完成"

    Rename-Item -Path "temp_build/$EXE_NAME" -NewName "QMediaSync_linux_arm64_exe" -Force
    Write-Colored "green" "重命名可执行文件为 QMediaSync_linux_arm64_exe"
} catch {
    Write-Colored "red" "Error: Build failed for linux/arm64"
    Write-Colored "red" $_.Exception.Message
    exit 1
}

Write-Colored "green" "========================================"
Write-Colored "green" "可执行文件编译完成！"
Write-Colored "green" "========================================"

Write-Colored "cyan" "Starting FPK application build for 飞牛系统..."

Write-Colored "cyan" "Building FPK for arm64 architecture..."

if (!(Test-Path -Path "FNOS" -PathType Container)) {
    Write-Colored "red" "Error: FNOS directory not found"
    Write-Colored "yellow" "Please create FNOS directory structure first"
} else {
    $arm64AppPath = "FNOS/qmediasync-arm64/app"
    New-Item -Path $arm64AppPath -ItemType Directory -Force | Out-Null

    $arm64ManifestPath = "FNOS/qmediasync-arm64/manifest"
    if (Test-Path -Path $arm64ManifestPath -PathType Leaf) {
        (Get-Content -Path $arm64ManifestPath) | ForEach-Object {
            if ($_ -match '^version\s*=') {
                "version = $TAG"
            } else {
                $_
            }
        } | Set-Content -Path $arm64ManifestPath -Force
        Write-Colored "green" "Updated version in FNOS/qmediasync-arm64/manifest to $TAG"
    } else {
        Write-Colored "yellow" "Warning: FNOS/qmediasync-arm64/manifest not found"
    }

    if (Test-Path -Path "$arm64AppPath/QMediaSync" -PathType Leaf) {
        Remove-Item -Path "$arm64AppPath/QMediaSync" -Force
        Write-Colored "yellow" "Removed existing arm64 executable"
    }

    if (Test-Path -Path "$arm64AppPath/web_statics" -PathType Container) {
        Remove-Item -Path "$arm64AppPath/web_statics" -Recurse -Force
        Write-Colored "yellow" "Removed existing web_statics directory"
    }

    if (Test-Path -Path "temp_build/QMediaSync_linux_arm64_exe" -PathType Leaf) {
        Copy-Item -Path "temp_build/QMediaSync_linux_arm64_exe" -Destination "$arm64AppPath/QMediaSync" -Force
        Write-Colored "green" "Copied arm64 executable to FNOS/qmediasync-arm64/app/QMediaSync"
    } else {
        Write-Colored "red" "Error: arm64 executable not found"
    }

    if (Test-Path -Path "web_statics" -PathType Container) {
        if (Test-Path -Path "assets/db_config.html" -PathType Leaf) {
            Copy-Item -Path "assets/db_config.html" -Destination "web_statics/" -Force
            Write-Colored "green" "Copied db_config.html from assets to web_statics"
        } else {
            Write-Colored "yellow" "Warning: assets/db_config.html not found"
        }
        Copy-Item -Path "web_statics" -Destination "$arm64AppPath/" -Recurse -Force
        Write-Colored "green" "Copied web_statics directory to FNOS/qmediasync-arm64/app/"
    } else {
        Write-Colored "yellow" "Warning: web_statics directory not found"
    }

    Set-Location -Path "FNOS/qmediasync-arm64"
    $fnpackExists = Get-Command "fnpack" -ErrorAction SilentlyContinue
    if ($fnpackExists) {
        Write-Colored "cyan" "Executing fnpack build for arm64..."
        fnpack build
        if ($LASTEXITCODE -eq 0) {
            Write-Colored "green" "FPK build completed for arm64"

            $ARM64_VERSION = $TAG.TrimStart('v')
            if (Test-Path -Path "qmediasync.fpk" -PathType Leaf) {
                Copy-Item -Path "qmediasync.fpk" -Destination "../../QMediaSync_arm64_$ARM64_VERSION.fpk" -Force
                Write-Colored "green" "Copied arm64 FPK file back to qmediasync directory"
            } else {
                Write-Colored "red" "Error: FPK file not generated for arm64"
            }
        } else {
            Write-Colored "red" "Error: fnpack build failed for arm64"
        }
    } else {
        Write-Colored "yellow" "Warning: fnpack command not found, skipping FPK build for arm64"
    }

    Set-Location -Path "../../"
}

Write-Colored "cyan" "Building FPK for amd64 architecture..."

if (!(Test-Path -Path "FNOS" -PathType Container)) {
    Write-Colored "red" "Error: FNOS directory not found"
    Write-Colored "yellow" "Please create FNOS directory structure first"
} else {
    $amd64AppPath = "FNOS/qmediasync-amd64/app"
    New-Item -Path $amd64AppPath -ItemType Directory -Force | Out-Null

    $amd64ManifestPath = "FNOS/qmediasync-amd64/manifest"
    if (Test-Path -Path $amd64ManifestPath -PathType Leaf) {
        (Get-Content -Path $amd64ManifestPath) | ForEach-Object {
            if ($_ -match '^version\s*=') {
                "version = $TAG"
            } else {
                $_
            }
        } | Set-Content -Path $amd64ManifestPath -Force
        Write-Colored "green" "Updated version in FNOS/qmediasync-amd64/manifest to $TAG"
    } else {
        Write-Colored "yellow" "Warning: FNOS/qmediasync-amd64/manifest not found"
    }

    if (Test-Path -Path "$amd64AppPath/QMediaSync" -PathType Leaf) {
        Remove-Item -Path "$amd64AppPath/QMediaSync" -Force
        Write-Colored "yellow" "Removed existing amd64 executable"
    }

    if (Test-Path -Path "$amd64AppPath/web_statics" -PathType Container) {
        Remove-Item -Path "$amd64AppPath/web_statics" -Recurse -Force
        Write-Colored "yellow" "Removed existing web_statics directory"
    }

    if (Test-Path -Path "temp_build/QMediaSync_linux_amd64_exe" -PathType Leaf) {
        Copy-Item -Path "temp_build/QMediaSync_linux_amd64_exe" -Destination "$amd64AppPath/QMediaSync" -Force
        Write-Colored "green" "Copied amd64 executable to FNOS/qmediasync-amd64/app/QMediaSync"
    } else {
        Write-Colored "red" "Error: amd64 executable not found"
    }

    if (Test-Path -Path "web_statics" -PathType Container) {
        if (Test-Path -Path "assets/db_config.html" -PathType Leaf) {
            Copy-Item -Path "assets/db_config.html" -Destination "web_statics/" -Force
            Write-Colored "green" "Copied db_config.html from assets to web_statics"
        } else {
            Write-Colored "yellow" "Warning: assets/db_config.html not found"
        }
        Copy-Item -Path "web_statics" -Destination "$amd64AppPath/" -Recurse -Force
        Write-Colored "green" "Copied web_statics directory to FNOS/qmediasync-amd64/app/"
    } else {
        Write-Colored "yellow" "Warning: web_statics directory not found"
    }

    Set-Location -Path "FNOS/qmediasync-amd64"
    $fnpackExists = Get-Command "fnpack" -ErrorAction SilentlyContinue
    if ($fnpackExists) {
        Write-Colored "cyan" "Executing fnpack build for amd64..."
        fnpack build
        if ($LASTEXITCODE -eq 0) {
            Write-Colored "green" "FPK build completed for amd64"

            $AMD64_VERSION = $TAG.TrimStart('v')
            if (Test-Path -Path "qmediasync.fpk" -PathType Leaf) {
                Copy-Item -Path "qmediasync.fpk" -Destination "../../QMediaSync_amd64_$AMD64_VERSION.fpk" -Force
                Write-Colored "green" "Copied amd64 FPK file back to qmediasync directory"
            } else {
                Write-Colored "red" "Error: FPK file not generated for amd64"
            }
        } else {
            Write-Colored "red" "Error: fnpack build failed for amd64"
        }
    } else {
        Write-Colored "yellow" "Warning: fnpack command not found, skipping FPK build for amd64"
    }

    Set-Location -Path "../../"
}

Write-Colored "green" "========================================"
Write-Colored "green" "FPK build process completed!"
Write-Colored "green" "========================================"

Write-Colored "cyan" "生成的FPK文件："
get-childitem -path "." -name "QMediaSync_*.fpk" | foreach-object {
    Write-Colored "green" "$_"
}

Write-Colored "green" "========================================"
Write-Colored "green" "所有操作完成！"
Write-Colored "green" "========================================"