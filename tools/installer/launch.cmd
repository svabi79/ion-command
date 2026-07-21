@echo off
REM ION COMMAND - starts the collector (if needed) and the client.
powershell -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "%~dp0launch.ps1" %*
