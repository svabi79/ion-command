param([string]$UnrealRoot = "", [switch]$SkipBootstrap)

$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$project = Join-Path $repositoryRoot 'unreal\IonCommand.uproject'
$engineRoot = & (Join-Path $PSScriptRoot 'find-unreal.ps1') -RequestedRoot $UnrealRoot
$editor = Join-Path $engineRoot 'Engine\Binaries\Win64\UnrealEditor.exe'
$editorCommand = Join-Path $engineRoot 'Engine\Binaries\Win64\UnrealEditor-Cmd.exe'
if (-not $SkipBootstrap) {
    $visualSourceScript = Join-Path $repositoryRoot 'unreal\Scripts\generate_visual_sources.py'
    & python $visualSourceScript
    if ($LASTEXITCODE -ne 0) { throw 'Visual source generation failed' }

    $scripts = 'generate_globe_mesh.py','create_material_instances.py','create_data_assets.py','create_bootstrap_level.py','validate_project.py'
    foreach ($scriptName in $scripts) {
        $scriptPath = Join-Path $repositoryRoot "unreal\Scripts\$scriptName"
        $scriptArgument = "-ExecutePythonScript=$scriptPath"
        $scriptStem = [IO.Path]::GetFileNameWithoutExtension($scriptName)
        $scriptLog = Join-Path $repositoryRoot "unreal\Saved\Logs\Automation-$scriptStem.log"
        $logArgument = "-AbsLog=$scriptLog"
        & $editorCommand $project -Unattended -NoSplash $scriptArgument $logArgument
        if ($LASTEXITCODE -ne 0) { throw "Unreal bootstrap script failed: $scriptName" }
        if (-not (Test-Path -LiteralPath $scriptLog)) { throw "Unreal bootstrap log was not created: $scriptLog" }
        $pythonErrors = Select-String -LiteralPath $scriptLog -Pattern 'LogPython: Error|LogEditorPythonExecuter: Error' -SimpleMatch:$false
        if ($pythonErrors) { throw "Unreal bootstrap script logged Python errors: $scriptName" }
    }
}
Start-Process -FilePath $editor -ArgumentList @("`"$project`"", '-log')
