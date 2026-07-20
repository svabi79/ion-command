#include "IonGlobeActor.h"

#include "Components/DirectionalLightComponent.h"
#include "Components/PointLightComponent.h"
#include "Components/SceneComponent.h"
#include "Components/StaticMeshComponent.h"
#include "Engine/Texture2D.h"
#include "GeoMathLibrary.h"
#include "GeoTimelineSubsystem.h"
#include "HttpModule.h"
#include "IImageWrapper.h"
#include "IImageWrapperModule.h"
#include "Interfaces/IHttpRequest.h"
#include "Interfaces/IHttpResponse.h"
#include "Materials/MaterialInstanceDynamic.h"
#include "Materials/MaterialInterface.h"
#include "Misc/CommandLine.h"
#include "Misc/Parse.h"
#include "Modules/ModuleManager.h"
#include "TimerManager.h"
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

    Clouds = CreateDefaultSubobject<UStaticMeshComponent>(TEXT("Clouds"));
    Clouds->SetupAttachment(SceneRoot);
    Clouds->SetStaticMesh(SphereMesh.Object);
    // Just above the surface, below markers (1008) and arcs.
    Clouds->SetRelativeScale3D(FVector(20.1));
    Clouds->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    Clouds->SetCastShadow(false);
    Clouds->SetVisibility(false);

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

    // Scattering-look shell preferred; the plain Fresnel instance stays as a
    // fallback for content states before the material scripts ran.
    UMaterialInterface* AtmosphereMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/M_AtmosphereScatter.M_AtmosphereScatter"));
    if (!AtmosphereMaterial) AtmosphereMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Atmosphere.MI_Atmosphere"));
    if (AtmosphereMaterial)
    {
        Atmosphere->SetMaterial(0, AtmosphereMaterial);
        Atmosphere->SetVisibility(true);
    }

    if (UMaterialInterface* CloudMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/M_CloudLayer.M_CloudLayer")))
    {
        Clouds->SetMaterial(0, CloudMaterial);
        Clouds->SetVisibility(true);
    }
}

void AIonGlobeActor::BeginPlay()
{
    Super::BeginPlay();
    if (!GetWorld() || !GetWorld()->IsGameWorld()) return;
    // Live weather: replace the packaged climatology cloud texture with the
    // current EUMETSAT world IR composite, refreshed hourly. -IonNoLiveClouds
    // keeps the static fallback (offline demos, deterministic captures).
    if (FParse::Param(FCommandLine::Get(), TEXT("IonNoLiveClouds"))) return;
    RequestLiveClouds();
    GetWorldTimerManager().SetTimer(CloudRefreshTimer, this, &AIonGlobeActor::RequestLiveClouds, 3600.0f, true);
}

void AIonGlobeActor::RequestLiveClouds()
{
    FString Url = TEXT("https://view.eumetsat.int/geoserver/wms?service=WMS&version=1.3.0&request=GetMap&layers=mumi:worldcloudmap_ir108&crs=EPSG:4326&bbox=-90,-180,90,180&width=2048&height=1024&format=image/png");
    FParse::Value(FCommandLine::Get(), TEXT("IonCloudUrl="), Url);
    const TSharedRef<IHttpRequest, ESPMode::ThreadSafe> Request = FHttpModule::Get().CreateRequest();
    Request->SetURL(Url);
    Request->SetVerb(TEXT("GET"));
    Request->SetTimeout(120.0f);
    TWeakObjectPtr<AIonGlobeActor> WeakThis(this);
    Request->OnProcessRequestComplete().BindLambda([WeakThis](FHttpRequestPtr, FHttpResponsePtr Response, bool bConnected)
    {
        if (!WeakThis.IsValid()) return;
        if (!bConnected || !Response.IsValid() || Response->GetResponseCode() != 200)
        {
            UE_LOG(LogTemp, Warning, TEXT("ION COMMAND live cloud fetch failed (%d); keeping current cloud texture"), Response.IsValid() ? Response->GetResponseCode() : -1);
            return;
        }
        WeakThis->ApplyLiveClouds(Response->GetContent());
    });
    Request->ProcessRequest();
}

void AIonGlobeActor::ApplyLiveClouds(const TArray<uint8>& PngBytes)
{
    IImageWrapperModule& Wrappers = FModuleManager::LoadModuleChecked<IImageWrapperModule>(FName("ImageWrapper"));
    const TSharedPtr<IImageWrapper> Png = Wrappers.CreateImageWrapper(EImageFormat::PNG);
    if (!Png.IsValid() || !Png->SetCompressed(PngBytes.GetData(), PngBytes.Num())) return;
    TArray64<uint8> Raw;
    if (!Png->GetRaw(ERGBFormat::BGRA, 8, Raw)) return;
    const int32 Width = Png->GetWidth();
    const int32 Height = Png->GetHeight();
    if (Width <= 0 || Height <= 0) return;
    TArray<FColor> Pixels;
    Pixels.SetNumUninitialized(Width * Height);
    for (int32 Y = 0; Y < Height; ++Y)
    {
        // IR cannot tell polar ice from cloud tops; damp high latitudes so
        // Antarctica does not render as a permanent storm.
        const double Latitude = 90.0 - (Y + 0.5) * 180.0 / Height;
        const double AbsLat = FMath::Abs(Latitude);
        const double PolarDamp = AbsLat <= 60.0 ? 1.0 : FMath::Max(0.1, FMath::Cos((AbsLat - 60.0) / 30.0 * HALF_PI));
        for (int32 X = 0; X < Width; ++X)
        {
            const int32 Index = Y * Width + X;
            const uint8 Value = Raw[Index * 4 + 2];
            // Contrast curve: IR mid-gray ocean background stays clear, only
            // genuinely cold (bright) cloud tops become opacity.
            double Fraction = FMath::Clamp((Value / 255.0 - 0.42) / 0.58, 0.0, 1.0);
            Fraction = FMath::Pow(Fraction, 1.35) * PolarDamp;
            const uint8 Gray = static_cast<uint8>(FMath::RoundToInt(Fraction * 255.0));
            Pixels[Index] = FColor(Gray, Gray, Gray, 255);
        }
    }
    if (!LiveCloudTexture || LiveCloudTexture->GetSizeX() != Width || LiveCloudTexture->GetSizeY() != Height)
    {
        LiveCloudTexture = UTexture2D::CreateTransient(Width, Height, PF_B8G8R8A8);
        if (!LiveCloudTexture) return;
        LiveCloudTexture->SRGB = false;
    }
    void* MipData = LiveCloudTexture->GetPlatformData()->Mips[0].BulkData.Lock(LOCK_READ_WRITE);
    FMemory::Memcpy(MipData, Pixels.GetData(), Pixels.Num() * sizeof(FColor));
    LiveCloudTexture->GetPlatformData()->Mips[0].BulkData.Unlock();
    LiveCloudTexture->UpdateResource();
    if (!CloudsMID) CloudsMID = Clouds->CreateAndSetMaterialInstanceDynamic(0);
    if (CloudsMID) CloudsMID->SetTextureParameterValue(TEXT("CloudMap"), LiveCloudTexture);
    UE_LOG(LogTemp, Display, TEXT("ION COMMAND live clouds applied (%dx%d, EUMETSAT world IR composite)"), Width, Height);
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
    if (!AtmosphereMID && Atmosphere->IsVisible())
    {
        AtmosphereMID = Atmosphere->CreateAndSetMaterialInstanceDynamic(0);
    }
    if (AtmosphereMID)
    {
        // Drives the day/terminator/night color bands of the scattering shell.
        AtmosphereMID->SetVectorParameterValue(TEXT("SunDirection"), FLinearColor(DirectionToSun.X, DirectionToSun.Y, DirectionToSun.Z, 0.0f));
    }
    if (Atmosphere->IsVisible()) Atmosphere->AddLocalRotation(FRotator(0.0, DeltaSeconds * 0.03, 0.0));
    // Clouds drift slowly relative to the terrain; the geographic frame of
    // arcs and markers stays pinned to the Earth mesh itself.
    if (Clouds->IsVisible()) Clouds->AddLocalRotation(FRotator(0.0, DeltaSeconds * 0.06, 0.0));
}
