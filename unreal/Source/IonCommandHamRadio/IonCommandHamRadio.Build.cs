using UnrealBuildTool;

public class IonCommandHamRadio : ModuleRules
{
    public IonCommandHamRadio(ReadOnlyTargetRules Target) : base(Target)
    {
        PCHUsage = PCHUsageMode.UseExplicitOrSharedPCHs;
        PublicDependencyModuleNames.AddRange(new[] { "Core", "CoreUObject", "Engine", "IonCommandCore", "IonCommandVisualization" });
    }
}

