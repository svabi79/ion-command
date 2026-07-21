@echo off
REM ION COMMAND - starts the collector (if needed) and the client.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0start-wall.ps1" %*
