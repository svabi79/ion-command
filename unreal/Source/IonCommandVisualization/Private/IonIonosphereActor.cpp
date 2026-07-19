#include "IonIonosphereActor.h"

#include "Components/SceneComponent.h"
#include "Components/StaticMeshComponent.h"
#include "Materials/MaterialInstanceDynamic.h"
#include "UObject/ConstructorHelpers.h"

AIonIonosphereActor::AIonIonosphereActor()
{
    PrimaryActorTick.bCanEverTick = true;
    SceneRoot = CreateDefaultSubobject<USceneComponent>(TEXT("Root")); SetRootComponent(SceneRoot);
    static ConstructorHelpers::FObjectFinder<UStaticMesh> SphereMesh(TEXT("/Engine/BasicShapes/Sphere.Sphere"));
    UMaterialInterface* ShellMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Ionosphere.MI_Ionosphere"));
    const double RadiusScales[] = {20.9, 21.6, 22.2, 23.1};
    for (int32 Index = 0; Index < 4; ++Index)
    {
        UStaticMeshComponent* Shell = CreateDefaultSubobject<UStaticMeshComponent>(*FString::Printf(TEXT("ConceptualShell_%d"), Index));
        Shell->SetupAttachment(SceneRoot); Shell->SetStaticMesh(SphereMesh.Object); Shell->SetRelativeScale3D(FVector(RadiusScales[Index]));
        Shell->SetCollisionEnabled(ECollisionEnabled::NoCollision); Shell->SetCastShadow(false); Shell->SetVisibility(ShellMaterial != nullptr);
        if (ShellMaterial) Shell->SetMaterial(0, ShellMaterial);
        Shells.Add(Shell);
    }
    // The conceptual shells start hidden so the showcase composition stays
    // clean; the operator toggles them with the I key.
    SetActorHiddenInGame(true);
}

void AIonIonosphereActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    for (int32 Index = 0; Index < Shells.Num(); ++Index) Shells[Index]->AddLocalRotation(FRotator(0, DeltaSeconds * (0.15 + Index * 0.04), 0));
}

