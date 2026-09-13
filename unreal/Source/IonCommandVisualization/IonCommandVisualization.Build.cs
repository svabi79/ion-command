using UnrealBuildTool;

public class IonCommandVisualization : ModuleRules
{
    public IonCommandVisualization(ReadOnlyTargetRules Target) : base(Target)
    {
        PCHUsage = PCHUsageMode.UseExplicitOrSharedPCHs;
        PublicDependencyModuleNames.AddRange(new[] { "Core", "CoreUObject", "Engine", "HTTP", "ProceduralMeshComponent", "IonCommandCore", "IonCommandData" });
        PrivateDependencyModuleNames.AddRange(new[] { "Niagara", "RenderCore", "RHI", "ImageWrapper" });
    }
}

