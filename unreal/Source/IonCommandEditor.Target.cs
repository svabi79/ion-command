using UnrealBuildTool;

public class IonCommandEditorTarget : TargetRules
{
    public IonCommandEditorTarget(TargetInfo Target) : base(Target)
    {
        Type = TargetType.Editor;
        DefaultBuildSettings = BuildSettingsVersion.V7;
        IncludeOrderVersion = EngineIncludeOrderVersion.Unreal5_8;
        ExtraModuleNames.Add("IonCommand");
    }
}
