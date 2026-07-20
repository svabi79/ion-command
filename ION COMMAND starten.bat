@echo off
REM Doppelklick startet Collector + Wall-Client (saubere Ansicht ohne Deck-Panels).
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0start-wall.ps1"
