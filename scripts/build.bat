@echo off
setlocal EnableDelayedExpansion
rem ============================================================================
rem  multi-terminals - Windows build batch
rem
rem  Windows (cmd.exe) port of scripts/build-all.sh. Builds the frontend in
rem  production mode, embeds it into the server binary, and produces the web
rem  server build and/or the Wails desktop build.
rem
rem  NOTE: keep this file ASCII-only and CRLF line endings. cmd.exe parses a
rem  batch file using the active code page; non-ASCII bytes corrupt parsing on
rem  Shift-JIS (cp932) consoles, and LF-only endings break "call :label".
rem
rem  Usage:
rem    scripts\build.bat [target]
rem
rem  target:
rem    all    web + wails (default)         -> release\
rem    web    web server binaries only (windows amd64/arm64, for release) -> release\
rem    wails  Wails desktop build only (windows/amd64) -> release\
rem    local  single runnable web binary with embedded UI (native arch) -> bin\
rem    start  run bin\multi-terminals.exe (web app on http://localhost:8080)
rem
rem  Notes:
rem    - Wails cannot cross-compile. The Windows build must run on Windows.
rem    - wails CLI required: go install github.com/wailsapp/wails/v2/cmd/wails@latest
rem    - all/web/wails write artifacts to release\; local writes bin\multi-terminals.exe.
rem ============================================================================

rem --- Move to the repository root so the script works from anywhere ---
set "SCRIPT_DIR=%~dp0"
pushd "%SCRIPT_DIR%.." || (echo error: cannot cd to repository root.& exit /b 1)
set "ROOT=%CD%"

set "TARGET=%~1"
if "%TARGET%"=="" set "TARGET=all"
set "OUTDIR=release"

rem --- Require go ---
where go >nul 2>&1
if errorlevel 1 (
  echo error: 'go' command not found. Install Go 1.26 or newer. 1>&2
  popd & exit /b 1
)

rem --- local / start: single runnable web binary in bin\ (no release artifacts) ---
if /i "%TARGET%"=="local" (
  call :build_local || goto :fail
  popd & exit /b 0
)
if /i "%TARGET%"=="start" (
  call :start_local || goto :fail
  popd & exit /b 0
)

if not exist "%OUTDIR%" mkdir "%OUTDIR%"

if /i "%TARGET%"=="web" (
  call :build_frontend || goto :fail
  call :build_web      || goto :fail
) else if /i "%TARGET%"=="wails" (
  call :build_frontend || goto :fail
  call :build_wails    || goto :fail
) else if /i "%TARGET%"=="all" (
  call :build_frontend || goto :fail
  call :build_web      || goto :fail
  call :build_wails    || goto :fail
) else (
  echo unknown target: %TARGET% ^(all^|web^|wails^|local^|start^) 1>&2
  popd & exit /b 1
)

call :write_checksums

echo.
echo [OK] build complete: %ROOT%\%OUTDIR%
dir /b "%OUTDIR%"
popd & exit /b 0

rem ============================================================================
rem  Build the frontend and place it into the embed directory
rem ============================================================================
:build_frontend
if not exist "frontend" (
  echo ^>^> no frontend: building without embedded UI
  exit /b 0
)
if not exist "frontend\node_modules" (
  echo ^>^> ^(cd frontend ^&^& npm install^)
  pushd frontend
  call npm install || (popd & exit /b 1)
  popd
)
echo ^>^> ^(cd frontend ^&^& npm run build^)
pushd frontend
call npm run build || (popd & exit /b 1)
popd
echo ^>^> embed frontend\dist -^> apps\web\webui\dist
if exist "apps\web\webui\dist" rmdir /s /q "apps\web\webui\dist"
mkdir "apps\web\webui\dist"
type nul > "apps\web\webui\dist\.gitkeep"
xcopy /e /i /q /y "frontend\dist\*" "apps\web\webui\dist\" >nul || exit /b 1
exit /b 0

rem ============================================================================
rem  Local single binary: bin\multi-terminals.exe with embedded UI (native arch).
rem  Equivalent to scripts/dev.sh build. Run it to serve the web app on :8080.
rem ============================================================================
:build_local
call :build_frontend || exit /b 1
if not exist "bin" mkdir "bin"
echo ^>^> go build -o bin\multi-terminals.exe ./apps/web/cmd
go build -o "bin\multi-terminals.exe" ./apps/web/cmd || exit /b 1
echo.
echo [OK] built %ROOT%\bin\multi-terminals.exe ^(single binary with embedded UI^)
echo    run:  scripts\build.bat start    ^(or  bin\multi-terminals.exe^)
echo    open: http://localhost:8080
exit /b 0

rem --- Run the local web binary (foreground; Ctrl-C to stop) ---
:start_local
if not exist "bin\multi-terminals.exe" (
  echo bin\multi-terminals.exe not found. Run: scripts\build.bat local 1>&2
  exit /b 1
)
echo ^>^> bin\multi-terminals.exe  ^(http://localhost:8080, Ctrl-C to stop^)
"bin\multi-terminals.exe"
exit /b 0

rem ============================================================================
rem  web server binaries (Windows amd64 / arm64)
rem ============================================================================
:build_web
echo ^>^> web: building for Windows
call :build_web_one amd64 multi-terminals-windows-amd64.exe || exit /b 1
call :build_web_one arm64 multi-terminals-windows-arm64.exe || exit /b 1
exit /b 0

:build_web_one
set "GOARCH_T=%~1"
set "OUT_T=%~2"
echo    - windows/%GOARCH_T% -^> %OUT_T%
set "CGO_ENABLED=0"
set "GOOS=windows"
set "GOARCH=%GOARCH_T%"
go build -trimpath -ldflags "-s -w" -o "%OUTDIR%\%OUT_T%" ./apps/web/cmd
set "BUILD_RC=%errorlevel%"
set "CGO_ENABLED="
set "GOOS="
set "GOARCH="
if not "%BUILD_RC%"=="0" exit /b 1
exit /b 0

rem ============================================================================
rem  Wails desktop build (windows/amd64)
rem ============================================================================
:build_wails
call :resolve_wails
if not defined WAILS (
  echo ^>^> [skip] wails CLI not installed; skipping Wails build. 1>&2
  echo    install: go install github.com/wailsapp/wails/v2/cmd/wails@latest 1>&2
  exit /b 0
)
echo ^>^> wails: building windows/amd64
pushd apps\wails
"%WAILS%" build -platform windows/amd64 -clean
set "WAILS_RC=%errorlevel%"
popd
if not "%WAILS_RC%"=="0" exit /b 1
if exist "apps\wails\build\bin" (
  echo ^>^> copy apps\wails\build\bin\* -^> %OUTDIR%\
  xcopy /e /i /q /y "apps\wails\build\bin\*" "%OUTDIR%\" >nul
) else (
  echo error: build artifact apps\wails\build\bin not found. 1>&2
  exit /b 1
)
exit /b 0

rem --- Resolve the wails CLI (PATH first, then GOBIN / GOPATH\bin) ---
:resolve_wails
set "WAILS="
where wails >nul 2>&1
if not errorlevel 1 (
  for /f "delims=" %%i in ('where wails') do (
    set "WAILS=%%i"
    goto :resolve_wails_done
  )
)
for /f "delims=" %%g in ('go env GOBIN 2^>nul') do set "GOBIN_DIR=%%g"
if defined GOBIN_DIR if exist "%GOBIN_DIR%\wails.exe" (
  set "WAILS=%GOBIN_DIR%\wails.exe"
  goto :resolve_wails_done
)
for /f "delims=" %%p in ('go env GOPATH 2^>nul') do set "GOPATH_DIR=%%p"
if defined GOPATH_DIR if exist "%GOPATH_DIR%\bin\wails.exe" (
  set "WAILS=%GOPATH_DIR%\bin\wails.exe"
  goto :resolve_wails_done
)
:resolve_wails_done
exit /b 0

rem ============================================================================
rem  Checksums (SHA256SUMS.txt) via PowerShell Get-FileHash.
rem  (certutil + for /f drops CR under the UTF-8 code page, so avoid it.)
rem  Output is sha256sum-compatible: lowercase hash + two spaces + file name.
rem ============================================================================
:write_checksums
echo ^>^> writing checksums: %OUTDIR%\SHA256SUMS.txt
powershell -NoProfile -ExecutionPolicy Bypass -Command "Get-ChildItem -File -LiteralPath '%OUTDIR%' | Where-Object { $_.Name -notmatch '^SHA256SUMS' -and $_.Name -ne 'NOTES.md' } | Sort-Object Name | ForEach-Object { '{0}  {1}' -f (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLower(), $_.Name } | Set-Content -Encoding ascii -LiteralPath '%OUTDIR%\SHA256SUMS.txt'"
type "%OUTDIR%\SHA256SUMS.txt"
exit /b 0

:fail
popd & exit /b 1
