using UnrealBuildTool;

public class IonCommandUI : ModuleRules
{
    public IonCommandUI(ReadOnlyTargetRules Target) : base(Target)
    {
        PCHUsage = PCHUsageMode.UseExplicitOrSharedPCHs;
        PublicDependencyModuleNames.AddRange(new[] { "Core", "CoreUObject", "Engine", "IonCommandCore", "IonCommandData" });
        PrivateDependencyModuleNames.AddRange(new[] { "UMG", "Slate", "SlateCore" });
    }
}

