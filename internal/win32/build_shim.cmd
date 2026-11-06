@echo off
rem Builds codmtune_ui.dll next to this script using the installed MSVC.
setlocal
call "E:\VSBuildTools\VC\Auxiliary\Build\vcvars64.bat" >nul 2>&1
if errorlevel 1 (
  echo vcvars64.bat not found
  exit /b 1
)
cd /d "%~dp0"
cl /nologo /LD /O2 /EHsc /MT /DUNICODE /D_UNICODE ui_shim.cpp ui_gpu.cpp ^
   /Fe:codmtune_ui.dll ^
   /link gdiplus.lib user32.lib gdi32.lib d2d1.lib d3d11.lib dxgi.lib dwrite.lib windowscodecs.lib ole32.lib
if errorlevel 1 exit /b 1
del /q ui_shim.obj ui_gpu.obj 2>nul
echo built codmtune_ui.dll
