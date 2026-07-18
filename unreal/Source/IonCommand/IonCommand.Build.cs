using UnrealBuildTool;

public class IonCommand : ModuleRules
{
    public IonCommand(ReadOnlyTargetRules Target) : base(Target)
    {
        PCHUsage = PCHUsageMode.UseExplicitOrSharedPCHs;
        PublicDependencyModuleNames.AddRange(new[] { "Core", "CoreUObject", "Engine", "InputCore", "IonCommandCore", "IonCommandData", "IonCommandVisualization", "IonCommandHamRadio", "IonCommandUI" });
    }
}
