#include "IonAuroraActor.h"

#include "Components/InstancedStaticMeshComponent.h"
#include "Components/SceneComponent.h"
#include "GeoMathLibrary.h"
#include "Materials/MaterialInstanceDynamic.h"
#include "UObject/ConstructorHelpers.h"

AIonAuroraActor::AIonAuroraActor()
{
    PrimaryActorTick.bCanEverTick = true;
    SceneRoot = CreateDefaultSubobject<USceneComponent>(TEXT("Root")); SetRootComponent(SceneRoot);
    static ConstructorHelpers::FObjectFinder<UStaticMesh> SphereMesh(TEXT("/Engine/BasicShapes/Sphere.Sphere"));
    NorthOval = CreateDefaultSubobject<UInstancedStaticMeshComponent>(TEXT("NorthAurora")); NorthOval->SetupAttachment(SceneRoot); NorthOval->SetStaticMesh(SphereMesh.Object); NorthOval->SetCollisionEnabled(ECollisionEnabled::NoCollision); NorthOval->SetCastShadow(false);
    SouthOval = CreateDefaultSubobject<UInstancedStaticMeshComponent>(TEXT("SouthAurora")); SouthOval->SetupAttachment(SceneRoot); SouthOval->SetStaticMesh(SphereMesh.Object); SouthOval->SetCollisionEnabled(ECollisionEnabled::NoCollision); SouthOval->SetCastShadow(false);
    if (UMaterialInterface* NorthMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Aurora_North.MI_Aurora_North"))) NorthOval->SetMaterial(0, NorthMaterial);
    if (UMaterialInterface* SouthMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Aurora_South.MI_Aurora_South"))) SouthOval->SetMaterial(0, SouthMaterial);
}

void AIonAuroraActor::OnConstruction(const FTransform& Transform)
{
    Super::OnConstruction(Transform);
    NorthOval->ClearInstances();
    SouthOval->ClearInstances();
    BuildOval(NorthOval, true);
    BuildOval(SouthOval, false);
}

void AIonAuroraActor::PostRegisterAllComponents()
{
    Super::PostRegisterAllComponents();
    if (GetWorld() && !GetWorld()->IsGameWorld())
    {
        NorthOval->ClearInstances();
        SouthOval->ClearInstances();
        BuildOval(NorthOval, true);
        BuildOval(SouthOval, false);
    }
}

void AIonAuroraActor::BeginPlay()
{
    Super::BeginPlay();
    NorthOval->ClearInstances();
    SouthOval->ClearInstances();
    if (UMaterialInstanceDynamic* Material = NorthOval->CreateAndSetMaterialInstanceDynamic(0)) Material->SetVectorParameterValue(TEXT("Color"), FLinearColor(0.02f, 1.0f, 0.38f));
    if (UMaterialInstanceDynamic* Material = SouthOval->CreateAndSetMaterialInstanceDynamic(0)) Material->SetVectorParameterValue(TEXT("Color"), FLinearColor(0.08f, 0.55f, 1.0f));
    BuildOval(NorthOval, true); BuildOval(SouthOval, false);
}

void AIonAuroraActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    NorthOval->AddLocalRotation(FRotator(0, DeltaSeconds * 0.30, 0));
    SouthOval->AddLocalRotation(FRotator(0, -DeltaSeconds * 0.22, 0));
}

void AIonAuroraActor::BuildOval(UInstancedStaticMeshComponent* Instances, bool bNorth)
{
    constexpr int32 Samples = 180;
    for (int32 Index = 0; Index < Samples; ++Index)
    {
        const double Longitude = -180.0 + Index * (360.0 / Samples);
        const double Wave = FMath::Sin(FMath::DegreesToRadians(Longitude * 3.0)) * 3.5;
        const double Latitude = (bNorth ? 69.0 : -69.0) + Wave;
        const FVector Location = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Latitude, Longitude) * 1032.0;
        const FQuat Rotation = FRotationMatrix::MakeFromZ(Location.GetSafeNormal()).ToQuat();
        const FVector Scale(0.20, 0.035, 0.42 + 0.14 * FMath::Sin(Index * 0.31));
        Instances->AddInstance(FTransform(Rotation, Location, Scale), true);
    }
}
