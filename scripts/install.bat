@echo off
REM Windows installer for taronja-gateway (tg)
REM Downloads the latest GoReleaser Windows artifact from GitHub and installs tg.exe to %USERPROFILE%\bin

setlocal enabledelayedexpansion

set REPO=jmaister/taronja-gateway
set TG_BIN=tg.exe
set INSTALL_DIR=%USERPROFILE%\bin
set API_URL=https://api.github.com/repos/%REPO%/releases/latest
set ZIP_URL=

REM Create install dir if it doesn't exist
if not exist "%INSTALL_DIR%" mkdir "%INSTALL_DIR%"

REM Get latest release info and extract the Windows tar.gz URL (GoReleaser artifact)
powershell -Command "Invoke-WebRequest -Uri '%API_URL%' -OutFile 'latest.json'"
for /f "delims=" %%i in ('powershell -Command "(Get-Content latest.json | Out-String | ConvertFrom-Json).assets | Where-Object { $_.name -like '*Windows_x86_64.tar.gz' } | Select-Object -ExpandProperty browser_download_url"') do set ZIP_URL=%%i

REM Debug: print ZIP_URL
if "%ZIP_URL%"=="" (
    echo ERROR: Could not find a Windows GoReleaser artifact download URL in the latest release.
    if exist latest.json type latest.json
    del latest.json
    exit /b 1
) else (
    echo Downloading from: %ZIP_URL%
)

REM Download the GoReleaser tar.gz artifact using PowerShell
powershell -Command "Invoke-WebRequest -Uri '%ZIP_URL%' -OutFile 'tg_win.tar.gz'"

REM Extract the tar.gz (GoReleaser puts binary in a subfolder)
set EXTRACT_DIR=%TEMP%\tg_extract
if exist "%EXTRACT_DIR%" rmdir /s /q "%EXTRACT_DIR%"
mkdir "%EXTRACT_DIR%"
tar -xzf tg_win.tar.gz -C "%EXTRACT_DIR%"

REM Find tg.exe recursively in the extracted folder
for /r "%EXTRACT_DIR%" %%F in (tg.exe) do (
    if exist "%%F" move /Y "%%F" "%INSTALL_DIR%\tg.exe"
)

REM Error if not found
if not exist "%INSTALL_DIR%\tg.exe" (
    echo ERROR: tg.exe not found after extraction. Please check the contents of %EXTRACT_DIR%.
)

REM Clean up
if exist tg_win.tar.gz del tg_win.tar.gz
if exist latest.json del latest.json
if exist "%EXTRACT_DIR%" rmdir /s /q "%EXTRACT_DIR%"

REM Add install dir to PATH if not already present
set PATH_CHECK=%PATH:;%INSTALL_DIR%;=%
if "%PATH_CHECK%"=="%PATH%" (
    echo.
    REM Show the current user PATH and what it will become
    set USERPATH=
    for /f "tokens=2*" %%A in ('reg query "HKCU\Environment" /v PATH 2^>nul') do set USERPATH=%%B
    if not defined USERPATH (
        REM User PATH is not set
        set NEWPATH=%INSTALL_DIR%
    ) else (
        REM User PATH exists
        set NEWPATH=!USERPATH!;%INSTALL_DIR%
    )
    REM Show only the new PATH that would be set
    echo.
    echo If you add automatically, your user PATH will become:
    echo !NEWPATH!
    echo.
    echo Adding %INSTALL_DIR% to your PATH to use 'tg.exe' from anywhere.
    set /p ADDPATH="Do you want to add it automatically with setx? (y/N): "
    if /I "!ADDPATH!"=="Y" (
        setx PATH "!NEWPATH!"
        echo %INSTALL_DIR% has been added to your user PATH. You may need to restart your command prompt for changes to take effect.
    )
) else (
    echo.
    echo 'tg.exe' is installed to %INSTALL_DIR% which is already in your PATH.
)

echo Done.
endlocal
