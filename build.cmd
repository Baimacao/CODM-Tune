@echo off
setlocal
cd /d "%~dp0"
if exist "E:\Go\bin\go.exe" set "PATH=E:\Go\bin;%PATH%"
if exist "E:\gopath\pkg\mod" set "GOPATH=E:\gopath"
if exist "E:\gopath\pkg\mod" set "GOMODCACHE=E:\gopath\pkg\mod"
set "GOPROXY=https://goproxy.cn,direct"

call internal\win32\build_shim.cmd
if errorlevel 1 exit /b 1

if not exist internal\win32\app.ico (
  echo missing internal\win32\app.ico
  exit /b 1
)

rem rsrc.syso is embedded into the exe; rebuilding only app.ico leaves every
rem Explorer/Alt-Tab icon at the previous size. Regenerate on each build.
"E:\GoPath\bin\rsrc.exe" -ico internal\win32\app.ico -o cmd\codmtune\rsrc.syso -arch amd64
if errorlevel 1 exit /b 1

go build -ldflags "-H=windowsgui" -o bin\codmtune.exe .\cmd\codmtune
if errorlevel 1 exit /b 1
go build -o bin\preview.exe .\cmd\preview
if errorlevel 1 exit /b 1
go build -o bin\verify.exe .\tools\verify
if errorlevel 1 exit /b 1

echo built bin\codmtune.exe
