@echo off
REM Windows installer for taronja-gateway (tg)
REM Downloads the latest release zip from GitHub and installs tg.exe to %USERPROFILE%\bin

setlocal
set REPO=jmaister/taronja-gateway
set TG_BIN=tg.exe
set INSTALL_DIR=%USERPROFILE%\bin
set API_URL=https://api.github.com/repos/%REPO%/releases/latest
set ZIP_NAME=
set ZIP_URL=

REM Create install dir if it doesn't exist
if not exist "%INSTALL_DIR%" mkdir "%INSTALL_DIR%"

REM Get latest release info and extract the Windows zip URL
for /f "tokens=*" %%i in ('curl -s %API_URL%') do echo %%i >> latest.json
for /f "tokens=2 delims=:," %%i in ('findstr /i "browser_download_url" latest.json ^| findstr /i "Windows_x86_64.zip"') do set ZIP_URL=%%i
set ZIP_URL=%ZIP_URL:~2,-1%

REM Download the zip
curl -L -o tg_win.zip "%ZIP_URL%"

REM Extract tg.exe
powershell -Command "Expand-Archive -Path tg_win.zip -DestinationPath ."

REM Move tg.exe to install dir
move /Y tg.exe "%INSTALL_DIR%\tg.exe"

REM Clean up
if exist tg_win.zip del tg_win.zip
if exist latest.json del latest.json

REM Add install dir to PATH if not already present
set PATH_CHECK=%PATH:;%INSTALL_DIR%;=%
if "%PATH_CHECK%"=="%PATH%" (
    echo.
    echo Add %INSTALL_DIR% to your PATH to use 'tg' from anywhere.
) else (
    echo.
    echo 'tg' installed to %INSTALL_DIR% and should be available in your PATH.
)

echo Done.
endlocal
