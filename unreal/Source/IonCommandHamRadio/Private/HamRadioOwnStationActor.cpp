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
    FGeoPosition Position;
    if (UGeoMathLibrary::MaidenheadToLatLon(ConfiguredLocator(), Position))
    {
        const FVector Location = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Position.Latitude, Position.Longitude) * (GlobeRadius + 12.0);
        SetActorLocation(Location);
    }
}

void AHamRadioOwnStationActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    PulsePhase += DeltaSeconds;
    const double Pulse = 0.6 + 0.4 * (0.5 + 0.5 * FMath::Sin(PulsePhase * 2.4));
    Halo->SetRelativeScale3D(FVector(0.45 + 0.25 * Pulse));
}
