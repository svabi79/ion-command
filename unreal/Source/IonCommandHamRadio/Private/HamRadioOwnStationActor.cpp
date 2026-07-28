#include "HamRadioOwnStationActor.h"

#include "Components/StaticMeshComponent.h"
#include "GeoMathLibrary.h"
#include "Materials/MaterialInterface.h"
#include "Misc/ConfigCacheIni.h"
#include "UObject/ConstructorHelpers.h"

AHamRadioOwnStationActor::AHamRadioOwnStationActor()
{
    PrimaryActorTick.bCanEverTick = true;
    SceneRoot = CreateDefaultSubobject<USceneComponent>(TEXT("Root"));
    SetRootComponent(SceneRoot);
    static ConstructorHelpers::FObjectFinder<UStaticMesh> SphereMesh(TEXT("/Engine/BasicShapes/Sphere.Sphere"));
    Marker = CreateDefaultSubobject<UStaticMeshComponent>(TEXT("Marker"));
    Marker->SetupAttachment(SceneRoot);
    Marker->SetStaticMesh(SphereMesh.Object);
    Marker->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    Marker->SetCastShadow(false);
    Marker->SetRelativeScale3D(FVector(0.24));
    if (UMaterialInterface* MarkerMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Signal_Selected.MI_Signal_Selected")))
    {
        Marker->SetMaterial(0, MarkerMaterial);
    }
    Halo = CreateDefaultSubobject<UStaticMeshComponent>(TEXT("Halo"));
    Halo->SetupAttachment(SceneRoot);
    Halo->SetStaticMesh(SphereMesh.Object);
    Halo->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    Halo->SetCastShadow(false);
    Halo->SetRelativeScale3D(FVector(0.6));
    if (UMaterialInterface* HaloMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Atmosphere.MI_Atmosphere")))
    {
        Halo->SetMaterial(0, HaloMaterial);
    }
}

FString AHamRadioOwnStationActor::ConfiguredCallsign()
{
    FString Callsign = TEXT("N0CALL");
    GConfig->GetString(TEXT("IonCommand.Station"), TEXT("Callsign"), Callsign, GGameIni);
    return Callsign;
}

TArray<FString> AHamRadioOwnStationActor::OwnStationEntityIds()
{
    const FString Callsign = ConfiguredCallsign();
    return {TEXT("hamradio:station:") + Callsign, TEXT("hamradio:receiver:") + Callsign};
}

FString AHamRadioOwnStationActor::ConfiguredLocator()
{
    FString Locator = TEXT("JN00AA");
    GConfig->GetString(TEXT("IonCommand.Station"), TEXT("Locator"), Locator, GGameIni);
    return Locator;
}

void AHamRadioOwnStationActor::BeginPlay()
{
    Super::BeginPlay();
    AppliedLocator = ConfiguredLocator();
    FGeoPosition Position;
    if (UGeoMathLibrary::MaidenheadToLatLon(AppliedLocator, Position))
    {
        const FVector Location = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Position.Latitude, Position.Longitude) * (GlobeRadius + 12.0);
        SetActorLocation(Location);
    }
}

void AHamRadioOwnStationActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    PulsePhase += DeltaSeconds;
    // Re-read the locator so a grid change in the settings panel repositions
    // the world halo live, in step with the HUD reticle (which reads the ini
    // every frame). Cheap config lookup; only moves when it actually changes.
    const FString CurrentLocator = ConfiguredLocator();
    if (CurrentLocator != AppliedLocator)
    {
        FGeoPosition Position;
        if (UGeoMathLibrary::MaidenheadToLatLon(CurrentLocator, Position))
        {
            SetActorLocation(UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Position.Latitude, Position.Longitude) * (GlobeRadius + 12.0));
            AppliedLocator = CurrentLocator;
        }
    }
    // Roughly constant screen size, like the point markers: the old fixed
    // 24/45-70 unit spheres ballooned into a small sun when zoomed in.
    double ZoomFactor = 1.0;
    if (const APlayerController* Player = GetWorld() ? GetWorld()->GetFirstPlayerController() : nullptr)
    {
        if (Player->PlayerCameraManager)
        {
            const double CameraDistance = Player->PlayerCameraManager->GetCameraLocation().Length();
            // Same floor as the point layer so the reticle keeps shrinking
            // through the close-orbit zoom range (effective minimum 0.0167 at
            // arm 1040; 0.012 is a safety margin below it, not a target).
            ZoomFactor = FMath::Clamp((CameraDistance - 1000.0) / 2400.0, 0.012, 1.15);
        }
    }
    const double Pulse = 0.6 + 0.4 * (0.5 + 0.5 * FMath::Sin(PulsePhase * 2.4));
    // Distinctly larger than a normal point marker (~0.2 x zoom) so "you are
    // here" reads at a glance, but zoom-scaled so it never blooms into a sun
    // when the operator zooms in (the old fixed 24/45-70 unit spheres did).
    Marker->SetRelativeScale3D(FVector(0.18 * ZoomFactor));
    Halo->SetRelativeScale3D(FVector((0.5 + 0.18 * Pulse) * ZoomFactor));
}
