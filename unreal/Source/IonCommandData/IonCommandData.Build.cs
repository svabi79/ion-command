using UnrealBuildTool;

public class IonCommandData : ModuleRules
{
    public IonCommandData(ReadOnlyTargetRules Target) : base(Target)
    {
        PCHUsage = PCHUsageMode.UseExplicitOrSharedPCHs;
        PublicDependencyModuleNames.AddRange(new[] { "Core", "CoreUObject", "Engine", "IonCommandCore", "WebSockets" });
        PrivateDependencyModuleNames.AddRange(new[] { "HTTP", "Json", "JsonUtilities" });
    }
}
