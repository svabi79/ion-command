; ION COMMAND - Windows installer (Inno Setup 6)
;
; Build with:
;   powershell -ExecutionPolicy Bypass -File tools\installer\build-installer.ps1
;
; The script expects a staged payload produced by build-installer.ps1 at
; {#StageDir}: the Shipping client under client\, the collector binary and its
; default config under collector\, plus launcher and docs.

#define AppName        "ION COMMAND"
#define AppPublisher   "Jan Svabenik (HB9HSJ)"
#define AppURL         "https://github.com/svabi79/ion-command"
#ifndef AppVersion
  #define AppVersion   "1.0.0"
#endif
#ifndef StageDir
  #define StageDir     "..\..\dist\installer-stage"
#endif
#ifndef OutDir
  #define OutDir       "..\..\dist\installer"
#endif

[Setup]
AppId={{8B6B0E4E-2F3E-4F8C-9E4B-3D1A6F5C7A21}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}/issues
AppUpdatesURL={#AppURL}/releases
DefaultDirName={autopf}\ION COMMAND
DefaultGroupName=ION COMMAND
DisableProgramGroupPage=yes
LicenseFile={#StageDir}\LICENSE.txt
InfoBeforeFile={#StageDir}\BEFORE-INSTALL.txt
OutputDir={#OutDir}
OutputBaseFilename=ION-COMMAND-{#AppVersion}-Setup
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
; The staged payload is ~450 MB; refuse to install without headroom.
ExtraDiskSpaceRequired=0
MinVersion=10.0
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayName={#AppName} {#AppVersion}
UninstallDisplayIcon={app}\client\IonCommand.exe
PrivilegesRequiredOverridesAllowed=dialog

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "german";  MessagesFile: "compiler:Languages\German.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: "firewall";    Description: "Allow the local data collector through Windows Firewall (loopback only)"; GroupDescription: "Network"; Flags: unchecked

[Files]
; Packaged Unreal client (Shipping). Debug symbols are excluded by the staging script.
Source: "{#StageDir}\client\*"; DestDir: "{app}\client"; Flags: ignoreversion recursesubdirs createallsubdirs
; Go collector plus its default configuration.
Source: "{#StageDir}\collector\ion-collector.exe"; DestDir: "{app}\collector"; Flags: ignoreversion
; The config is user-editable: install it only if absent so upgrades keep local edits.
Source: "{#StageDir}\collector\configs\live.json"; DestDir: "{app}\collector\configs"; Flags: onlyifdoesntexist uninsneveruninstall
; Launcher and documentation.
Source: "{#StageDir}\launch.ps1"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#StageDir}\launch.cmd"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#StageDir}\README.md";  DestDir: "{app}"; Flags: ignoreversion isreadme
Source: "{#StageDir}\LICENSE.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#StageDir}\docs\*"; DestDir: "{app}\docs"; Flags: ignoreversion recursesubdirs createallsubdirs skipifsourcedoesntexist

[Icons]
Name: "{group}\ION COMMAND";            Filename: "{app}\launch.cmd"; WorkingDir: "{app}"; IconFilename: "{app}\client\IonCommand.exe"; Comment: "Start the collector and the globe"
Name: "{group}\ION COMMAND (windowed)"; Filename: "{app}\launch.cmd"; Parameters: "-ResX 1920 -ResY 1080"; WorkingDir: "{app}"; IconFilename: "{app}\client\IonCommand.exe"
Name: "{group}\Configuration folder";   Filename: "{app}\collector\configs"
Name: "{group}\Documentation";          Filename: "{app}\README.md"
Name: "{group}\{cm:UninstallProgram,ION COMMAND}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\ION COMMAND";      Filename: "{app}\launch.cmd"; WorkingDir: "{app}"; IconFilename: "{app}\client\IonCommand.exe"; Tasks: desktopicon

[Run]
; Loopback-only rule: the collector serves 127.0.0.1:7810 for the local client.
Filename: "netsh"; Parameters: "advfirewall firewall add rule name=""ION COMMAND collector (loopback)"" dir=in action=allow program=""{app}\collector\ion-collector.exe"" enable=yes profile=any localip=127.0.0.1 remoteip=127.0.0.1"; Flags: runhidden; Tasks: firewall; StatusMsg: "Adding firewall rule..."
Filename: "{app}\launch.cmd"; Description: "{cm:LaunchProgram,ION COMMAND}"; WorkingDir: "{app}"; Flags: nowait postinstall skipifsilent shellexec

[UninstallRun]
Filename: "netsh"; Parameters: "advfirewall firewall delete rule name=""ION COMMAND collector (loopback)"""; Flags: runhidden; RunOnceId: "DelFirewallRule"

[UninstallDelete]
; Runtime state the app creates outside the install tree.
Type: filesandordirs; Name: "{localappdata}\IonCommand\logs"

[Code]
// Stop a running instance before installing or uninstalling, otherwise the
// packaged client and collector files are locked and the copy fails.
procedure StopRunningInstances;
var
  ResultCode: Integer;
begin
  Exec('taskkill.exe', '/F /IM IonCommand.exe', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec('taskkill.exe', '/F /IM ion-collector.exe', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  StopRunningInstances;
  Result := '';
end;

function InitializeUninstall(): Boolean;
begin
  StopRunningInstances;
  Result := True;
end;
