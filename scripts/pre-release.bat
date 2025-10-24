@echo off
REM Pre-Release Validation Script for Relica (Windows)
REM This script simply calls the bash version using Git Bash

REM Check if bash is available (Git Bash)
where bash >nul 2>nul
if errorlevel 1 (
    echo [ERROR] bash not found. Please install Git for Windows.
    echo Download: https://git-scm.com/download/win
    exit /b 1
)

REM Run the bash script
bash %~dp0pre-release.sh
exit /b %ERRORLEVEL%
