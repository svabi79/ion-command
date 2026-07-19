#include "IonAuroraActor.h"

#include "Components/InstancedStaticMeshComponent.h"
#include "Components/SceneComponent.h"
#include "GeoDataSubsystem.h"
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
    if (UMaterialInterface* NorthMaterialAsset = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Aurora_North.MI_Aurora_North"))) NorthOval->SetMaterial(0, NorthMaterialAsset);
    if (UMaterialInterface* SouthMaterialAsset = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Aurora_South.MI_Aurora_South"))) SouthOval->SetMaterial(0, SouthMaterialAsset);
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
    NorthMaterial = NorthOval->CreateAndSetMaterialInstanceDynamic(0);
    if (NorthMaterial) NorthMaterial->SetVectorParameterValue(TEXT("Color"), FLinearColor(0.02f, 1.0f, 0.38f));
    SouthMaterial = SouthOval->CreateAndSetMaterialInstanceDynamic(0);
    if (SouthMaterial) SouthMaterial->SetVectorParameterValue(TEXT("Color"), FLinearColor(0.08f, 0.55f, 1.0f));
    BuildOval(NorthOval, true); BuildOval(SouthOval, false);
    BuiltKp = CurrentKp;
    if (UGameInstance* GameInstance = GetGameInstance())
    {
        DataSubsystem = GameInstance->GetSubsystem<UGeoDataSubsystem>();
        if (DataSubsystem.IsValid())
        {
            DataSubsystem->OnMessageAccepted().AddUObject(this, &AIonAuroraActor::OnMessageAccepted);
        }
    }
}

void AIonAuroraActor::EndPlay(const EEndPlayReason::Type EndPlayReason)
{
    if (DataSubsystem.IsValid()) DataSubsystem->OnMessageAccepted().RemoveAll(this);
    Super::EndPlay(EndPlayReason);
}

void AIonAuroraActor::OnMessageAccepted(const FGeoMessageEnvelope& Message)
{
    if (Message.SemanticType != TEXT("spaceweather.state")) return;
    const FString KpValue = Message.Properties.FindRef(TEXT("kp"));
    if (KpValue.IsEmpty()) return;
    CurrentKp = FMath::Clamp(FCString::Atod(*KpValue), 0.0, 9.0);
}

void AIonAuroraActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    NorthOval->AddLocalRotation(FRotator(0, DeltaSeconds * 0.30, 0));
    SouthOval->AddLocalRotation(FRotator(0, -DeltaSeconds * 0.22, 0));
    if (FMath::Abs(CurrentKp - BuiltKp) > 0.3)
    {
        BuiltKp = CurrentKp;
        NorthOval->ClearInstances();
        SouthOval->ClearInstances();
        BuildOval(NorthOval, true);
        BuildOval(SouthOval, false);
        const float Intensity = static_cast<float>(1.0 + 0.35 * CurrentKp);
        if (NorthMaterial) NorthMaterial->SetScalarParameterValue(TEXT("Intensity"), 1.6f * Intensity);
        if (SouthMaterial) SouthMaterial->SetScalarParameterValue(TEXT("Intensity"), 1.4f * Intensity);
    }
}

void AIonAuroraActor::BuildOval(UInstancedStaticMeshComponent* Instances, bool bNorth)
{
    // Many small, flat, overlapping segments read as a continuous curtain
    // instead of a chain of pearls. The oval expands equatorward with Kp.
    const double CenterLatitude = FMath::Clamp(71.0 - 2.2 * CurrentKp, 48.0, 74.0);
    constexpr int32 Samples = 320;
    for (int32 Index = 0; Index < Samples; ++Index)
    {
        const double Longitude = -180.0 + Index * (360.0 / Samples);
        const double Wave = FMath::Sin(FMath::DegreesToRadians(Longitude * 3.0)) * 3.5;
        const double Latitude = (bNorth ? CenterLatitude : -CenterLatitude) + Wave;
        const FVector Location = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Latitude, Longitude) * 1030.0;
        const FQuat Rotation = FRotationMatrix::MakeFromZ(Location.GetSafeNormal()).ToQuat();
        const FVector Scale(0.30, 0.016, 0.20 + 0.07 * FMath::Sin(Index * 0.31));
        Instances->AddInstance(FTransform(Rotation, Location, Scale), true);
    }
}
