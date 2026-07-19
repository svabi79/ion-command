#include "IonGlobeActor.h"

#include "Components/DirectionalLightComponent.h"
#include "Components/PointLightComponent.h"
#include "Components/SceneComponent.h"
#include "Components/StaticMeshComponent.h"
#include "GeoMathLibrary.h"
#include "GeoTimelineSubsystem.h"
#include "Materials/MaterialInstanceDynamic.h"
#include "Materials/MaterialInterface.h"
#include "UObject/ConstructorHelpers.h"

AIonGlobeActor::AIonGlobeActor()
{
    PrimaryActorTick.bCanEverTick = true;
    SceneRoot = CreateDefaultSubobject<USceneComponent>(TEXT("Root"));
    SetRootComponent(SceneRoot);

    static ConstructorHelpers::FObjectFinder<UStaticMesh> SphereMesh(TEXT("/Engine/BasicShapes/Sphere.Sphere"));

    Starfield = CreateDefaultSubobject<UStaticMeshComponent>(TEXT("Starfield"));
    Starfield->SetupAttachment(SceneRoot);
    Starfield->SetStaticMesh(SphereMesh.Object);
    Starfield->SetRelativeScale3D(FVector(180.0));
    Starfield->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    Starfield->SetCastShadow(false);
    Starfield->SetReceivesDecals(false);
    Starfield->SetVisibility(false);

    Earth = CreateDefaultSubobject<UStaticMeshComponent>(TEXT("Earth"));
    Earth->SetupAttachment(SceneRoot);
    Earth->SetStaticMesh(SphereMesh.Object);
    Earth->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    Earth->SetRelativeScale3D(FVector(20.0));

    Atmosphere = CreateDefaultSubobject<UStaticMeshComponent>(TEXT("Atmosphere"));
    Atmosphere->SetupAttachment(SceneRoot);
    Atmosphere->SetStaticMesh(SphereMesh.Object);
    Atmosphere->SetRelativeScale3D(FVector(20.7));
    Atmosphere->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    Atmosphere->SetCastShadow(false);
    Atmosphere->SetVisibility(false);

    SunLight = CreateDefaultSubobject<UDirectionalLightComponent>(TEXT("Sun"));
    SunLight->SetupAttachment(SceneRoot);
    SunLight->SetIntensity(5.5f);
    SunLight->SetLightColor(FLinearColor(1.0f, 0.93f, 0.82f));
    SunLight->SetCastShadows(true);
    SunLight->ForwardShadingPriority = 1;

    RimLight = CreateDefaultSubobject<UDirectionalLightComponent>(TEXT("RimLight"));
    RimLight->SetupAttachment(SceneRoot);
    RimLight->SetRelativeRotation(FRotator(-28.0, 155.0, 0.0));
    RimLight->SetIntensity(1.2f);
    RimLight->SetLightColor(FLinearColor(0.02f, 0.25f, 1.0f));
    RimLight->SetCastShadows(false);

    CoreGlow = CreateDefaultSubobject<UPointLightComponent>(TEXT("CoreGlow"));
    CoreGlow->SetupAttachment(SceneRoot);
    CoreGlow->SetIntensity(8000.0f);
    CoreGlow->SetAttenuationRadius(2400.0f);
    CoreGlow->SetLightColor(FLinearColor(0.0f, 0.18f, 0.32f));

    if (UMaterialInterface* EarthMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/M_EarthSurface.M_EarthSurface")))
    {
        Earth->SetMaterial(0, EarthMaterial);
    }

    if (UMaterialInterface* StarMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/M_Starfield.M_Starfield")))
    {
        Starfield->SetMaterial(0, StarMaterial);
        Starfield->SetVisibility(true);
    }

    if (UMaterialInterface* AtmosphereMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Atmosphere.MI_Atmosphere")))
    {
        Atmosphere->SetMaterial(0, AtmosphereMaterial);
        Atmosphere->SetVisibility(true);
    }
}

void AIonGlobeActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    const UGeoTimelineSubsystem* Timeline = GetGameInstance() ? GetGameInstance()->GetSubsystem<UGeoTimelineSubsystem>() : nullptr;
    const FDateTime TimelineUtc = Timeline ? Timeline->GetTimelineUtc() : FDateTime::UtcNow();
    const FGeoPosition Subsolar = UGeoMathLibrary::SolarSubpoint(TimelineUtc);
    const FVector DirectionToSun = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Subsolar.Latitude, Subsolar.Longitude);
    SunLight->SetWorldRotation((-DirectionToSun).Rotation());
    if (!EarthMID)
    {
        EarthMID = Earth->CreateAndSetMaterialInstanceDynamic(0);
    }
    if (EarthMID)
    {
        // The material masks the city-light emissive to the true night side.
        EarthMID->SetVectorParameterValue(TEXT("SunDirection"), FLinearColor(DirectionToSun.X, DirectionToSun.Y, DirectionToSun.Z, 0.0f));
    }
    if (Atmosphere->IsVisible()) Atmosphere->AddLocalRotation(FRotator(0.0, DeltaSeconds * 0.03, 0.0));
}
