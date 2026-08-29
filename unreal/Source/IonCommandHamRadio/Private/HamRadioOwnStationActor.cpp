#include "HamRadioOwnStationActor.h"

#include "Components/StaticMeshComponent.h"
#include "GeoMathLibrary.h"
#include "Materials/MaterialInterface.h"
#include "IonOperatorConfig.h"
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
    IonOperatorConfig::GetString(TEXT("IonCommand.Station"), TEXT("Callsign"), Callsign);
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
    IonOperatorConfig::GetString(TEXT("IonCommand.Station"), TEXT("Locator"), Locator);
    return Locator;
}

bool AHamRadioOwnStationActor::IsStationConfigured()
{
    // The shipped defaults are a placeholder, not a location: N0CALL is the
    // reserved "no callsign" token and JN00AA is a real grid square on the
    // Spanish coast. Drawing a marker there would assert an operator position
    // that is simply wrong, so an unconfigured station shows nothing at all.
    return ConfiguredCallsign() != TEXT("N0CALL") && ConfiguredLocator() != TEXT("JN00AA");
}

void AHamRadioOwnStationActor::BeginPlay()
{
    Super::BeginPlay();
    if (!IsStationConfigured())
    {
        SetActorHiddenInGame(true);
        UE_LOG(LogTemp, Warning, TEXT("ION COMMAND own station: not configured (callsign=%s locator=%s); marker hidden. Set it in the settings panel."),
            *ConfiguredCallsign(), *ConfiguredLocator());
        return;
    }
    AppliedLocator = ConfiguredLocator();
    FGeoPosition Position;
    if (UGeoMathLibrary::MaidenheadToLatLon(AppliedLocator, Position))
    {
        const FVector Location = PlaceOnGlobe(Position, MarkerAltitude());
        SetActorLocation(Location);
        SetActorHiddenInGame(false);
        // Anchor the station in the saved config. A hand-written Game.ini does
        // NOT survive a run: Unreal rewrites the saved hierarchy on shutdown
        // and keeps only values that went through GConfig, so the file is
        // deleted and the next start silently falls back to the placeholder.
        // Writing the values back once makes an externally provisioned station
        // stick.
        // Anchor the station in the operator ini. A station provisioned in
        // the engine's own Game ini (by hand, or by an installer) would be
        // dropped on shutdown; copying it here once makes it permanent.
        IonOperatorConfig::SetString(TEXT("IonCommand.Station"), TEXT("Callsign"), ConfiguredCallsign());
        IonOperatorConfig::SetString(TEXT("IonCommand.Station"), TEXT("Locator"), AppliedLocator);
        // One line at startup so "my station sits in the wrong place" can be
        // diagnosed from a log instead of by eye on a cluttered globe.
        UE_LOG(LogTemp, Display, TEXT("ION COMMAND own station: callsign=%s locator=%s -> lat=%.4f lon=%.4f world=(%.1f, %.1f, %.1f)"),
            *ConfiguredCallsign(), *AppliedLocator, Position.Latitude, Position.Longitude, Location.X, Location.Y, Location.Z);
    }
    else
    {
        SetActorHiddenInGame(true);
        UE_LOG(LogTemp, Warning, TEXT("ION COMMAND own station: locator %s could not be parsed; marker hidden"), *AppliedLocator);
    }
}

FVector AHamRadioOwnStationActor::PlaceOnGlobe(const FGeoPosition& Position, double Altitude) const
{
    return UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Position.Latitude, Position.Longitude) * (GlobeRadius + Altitude);
}

double AHamRadioOwnStationActor::MarkerAltitude() const
{
    // 12 units is 76 km off the surface - fine from orbit, and above the
    // camera itself at the closest approach (~32 km), where the marker ends
    // up behind the viewer and vanishes exactly when the operator zooms in
    // to look at it. Keep it a quarter of the way up to the camera instead,
    // so it always stands between the ground and the eye.
    constexpr double OrbitAltitude = 12.0;
    if (const APlayerController* Player = GetWorld() ? GetWorld()->GetFirstPlayerController() : nullptr)
    {
        if (Player->PlayerCameraManager)
        {
            const double CameraAltitude = Player->PlayerCameraManager->GetCameraLocation().Length() - GlobeRadius;
            if (CameraAltitude > 0.0)
            {
                return FMath::Min(OrbitAltitude, CameraAltitude * 0.25);
            }
        }
    }
    return OrbitAltitude;
}

void AHamRadioOwnStationActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    PulsePhase += DeltaSeconds;
    // Re-place every frame: the altitude tracks the camera, so a marker set
    // once at BeginPlay would be correct only at the distance it started at.
    if (!AppliedLocator.IsEmpty() && IsStationConfigured())
    {
        FGeoPosition Placed;
        if (UGeoMathLibrary::MaidenheadToLatLon(AppliedLocator, Placed))
        {
            SetActorLocation(PlaceOnGlobe(Placed, MarkerAltitude()));
        }
    }
    // Re-read the locator so a grid change in the settings panel repositions
    // the world halo live, in step with the HUD reticle (which reads the ini
    // every frame). Cheap config lookup; only moves when it actually changes.
    const FString CurrentLocator = ConfiguredLocator();
    if (CurrentLocator != AppliedLocator)
    {
        FGeoPosition Position;
        if (IsStationConfigured() && UGeoMathLibrary::MaidenheadToLatLon(CurrentLocator, Position))
        {
            SetActorLocation(PlaceOnGlobe(Position, MarkerAltitude()));
            SetActorHiddenInGame(false);
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
            // through the close-orbit zoom range (effective minimum 0.0021 at
            // the closest approach of arm 1005; the floor is a safety margin
            // below it, not a target). Keep both in step.
            ZoomFactor = FMath::Clamp((CameraDistance - 1000.0) / 2400.0, 0.0015, 1.15);
        }
    }
    const double Pulse = 0.6 + 0.4 * (0.5 + 0.5 * FMath::Sin(PulsePhase * 2.4));
    // Distinctly larger than a normal point marker (~0.2 x zoom) so "you are
    // here" reads at a glance, but zoom-scaled so it never blooms into a sun
    // when the operator zooms in (the old fixed 24/45-70 unit spheres did).
    Marker->SetRelativeScale3D(FVector(0.18 * ZoomFactor));
    Halo->SetRelativeScale3D(FVector((0.5 + 0.18 * Pulse) * ZoomFactor));
}
