@echo off
setlocal
cd /d "%~dp0"
if not exist "data\normal" mkdir "data\normal"
echo Starting ZION v0.1.0-alpha.1 NORMAL node...
"%~dp0zion-node.exe" run --config "%~dp0configs\normal.yaml" --data-dir "%~dp0data\normal"
set "ZION_EXIT=%ERRORLEVEL%"
if not "%ZION_EXIT%"=="0" (
  echo.
  echo ZION stopped with exit code %ZION_EXIT%. Review the error above.
  pause
)
exit /b %ZION_EXIT%

