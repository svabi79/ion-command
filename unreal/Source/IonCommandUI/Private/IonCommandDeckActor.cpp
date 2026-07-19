#include "IonCommandDeckActor.h"

#include "Components/SceneComponent.h"
#include "Components/StaticMeshComponent.h"
#include "Components/TextRenderComponent.h"
#include "GeoDataSubsystem.h"
#include "GeoMathLibrary.h"
#include "GeoSelectionSubsystem.h"
#include "GeoStreamSubsystem.h"
#include "GeoTimelineSubsystem.h"
#include "Materials/MaterialInstanceDynamic.h"
#include "Misc/ConfigCacheIni.h"
#include "UObject/ConstructorHelpers.h"

AIonCommandDeckActor::AIonCommandDeckActor()
{
    PrimaryActorTick.bCanEverTick = true;
    PrimaryActorTick.TickInterval = 0.25f;
    SceneRoot = CreateDefaultSubobject<USceneComponent>(TEXT("Root")); SetRootComponent(SceneRoot);
    static ConstructorHelpers::FObjectFinder<UStaticMesh> CubeMesh(TEXT("/Engine/BasicShapes/Cube.Cube"));
    UMaterialInterface* ConsoleMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Console.MI_Console"));
    const TArray<FVector> Locations = {FVector(0, -1450, 0), FVector(60, 0, -820), FVector(0, 1450, 0)};
    const TArray<FRotator> Rotations = {FRotator(2, -12, -3), FRotator(8, 0, 0), FRotator(2, 12, 3)};
    const TArray<FVector> Scales = {FVector(0.10, 5.8, 2.2), FVector(0.10, 8.5, 1.7), FVector(0.10, 5.8, 2.2)};
    for (int32 Index = 0; Index < 3; ++Index)
    {
        UStaticMeshComponent* Surface = CreateDefaultSubobject<UStaticMeshComponent>(*FString::Printf(TEXT("ConsoleSurface_%d"), Index));
        Surface->SetupAttachment(SceneRoot); Surface->SetStaticMesh(CubeMesh.Object);
        if (ConsoleMaterial) Surface->SetMaterial(0, ConsoleMaterial);
        Surface->SetRelativeLocation(Locations[Index]); Surface->SetRelativeRotation(Rotations[Index]); Surface->SetRelativeScale3D(Scales[Index]);
        Surface->SetCollisionEnabled(ECollisionEnabled::QueryOnly); ConsoleSurfaces.Add(Surface);
    }
    TitleText = CreateDefaultSubobject<UTextRenderComponent>(TEXT("Title")); TitleText->SetupAttachment(SceneRoot); TitleText->SetRelativeLocation(FVector(-35, 0, 910)); TitleText->SetRelativeRotation(FRotator(0, 180, 0)); TitleText->SetHorizontalAlignment(EHTA_Center); TitleText->SetWorldSize(52); TitleText->SetTextRenderColor(FColor(40, 220, 255)); TitleText->SetText(FText::FromString(TEXT("ION COMMAND // GEOSPATIAL OPERATIONS")));
    StreamText = CreateDefaultSubobject<UTextRenderComponent>(TEXT("StreamStatus")); StreamText->SetupAttachment(SceneRoot); StreamText->SetRelativeLocation(FVector(-35, -1450, 10)); StreamText->SetRelativeRotation(FRotator(0, 180, 0)); StreamText->SetHorizontalAlignment(EHTA_Center); StreamText->SetWorldSize(34); StreamText->SetTextRenderColor(FColor(60, 255, 150)); StreamText->SetText(FText::FromString(TEXT("LIVE LINK // STANDBY")));
    TelemetryText = CreateDefaultSubobject<UTextRenderComponent>(TEXT("Telemetry")); TelemetryText->SetupAttachment(SceneRoot); TelemetryText->SetRelativeLocation(FVector(-35, 1450, 10)); TelemetryText->SetRelativeRotation(FRotator(0, 180, 0)); TelemetryText->SetHorizontalAlignment(EHTA_Center); TelemetryText->SetWorldSize(30); TelemetryText->SetTextRenderColor(FColor(30, 190, 255)); TelemetryText->SetText(FText::FromString(TEXT("ACTIVE 000000  //  RX 000000000\nDROP 000000")));
    SelectionText = CreateDefaultSubobject<UTextRenderComponent>(TEXT("SelectionDetails")); SelectionText->SetupAttachment(SceneRoot); SelectionText->SetRelativeLocation(FVector(-55, 0, -810)); SelectionText->SetRelativeRotation(FRotator(0, 180, 0)); SelectionText->SetHorizontalAlignment(EHTA_Center); SelectionText->SetVerticalAlignment(EVRTA_TextCenter); SelectionText->SetWorldSize(25); SelectionText->SetTextRenderColor(FColor(220, 252, 255)); SelectionText->SetText(FText::FromString(TEXT("SELECT PATH // LMB\nORBIT // RMB   ZOOM // WHEEL\nPAUSE // SPACE   RETURN LIVE // L")));
}

void AIonCommandDeckActor::BeginPlay()
{
    Super::BeginPlay();
    FString ProductName = TEXT("ION COMMAND");
    FString Subtitle = TEXT("Global Geospatial Operations");
    GConfig->GetString(TEXT("/Script/EngineSettings.GeneralProjectSettings"), TEXT("ProjectName"), ProductName, GGameIni);
    GConfig->GetString(TEXT("/Script/EngineSettings.GeneralProjectSettings"), TEXT("Description"), Subtitle, GGameIni);
    TitleText->SetText(FText::FromString(ProductName + TEXT(" // ") + Subtitle));
}

void AIonCommandDeckActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    if (!GetGameInstance()) return;
    // Diegetic boot readout during the camera fade-in; selection input skips
    // straight to the operational readout.
    const double BootSeconds = GetWorld() ? GetWorld()->GetTimeSeconds() : 10.0;
    const UGeoSelectionSubsystem* BootSelection = GetGameInstance()->GetSubsystem<UGeoSelectionSubsystem>();
    if (BootSeconds < 5.0 && (!BootSelection || !BootSelection->HasSelection()))
    {
        static const TCHAR* BootLines[] = {
            TEXT("INITIALISING RENDERER"),
            TEXT("LOADING EARTH MODEL"),
            TEXT("CONNECTING TELEMETRY"),
            TEXT("SYNCHRONISING UTC"),
        };
        const int32 BootLine = FMath::Clamp(static_cast<int32>(BootSeconds / 1.25), 0, 3);
        SelectionText->SetText(FText::FromString(FString::Printf(TEXT(">> %s"), BootLines[BootLine])));
        SelectionText->SetTextRenderColor(FColor(70, 210, 255));
        return;
    }
    const UGeoStreamSubsystem* Stream = GetGameInstance()->GetSubsystem<UGeoStreamSubsystem>();
    const UGeoDataSubsystem* Data = GetGameInstance()->GetSubsystem<UGeoDataSubsystem>();
    const UGeoTimelineSubsystem* Timeline = GetGameInstance()->GetSubsystem<UGeoTimelineSubsystem>();
    const UGeoSelectionSubsystem* Selection = GetGameInstance()->GetSubsystem<UGeoSelectionSubsystem>();
    FString State = TEXT("OFFLINE");
    FColor StateColor(255, 140, 30);
    if (Stream)
    {
        switch (Stream->GetState())
        {
        case EGeoStreamState::Connected: State = TEXT("LIVE LINK // CONNECTED"); StateColor = FColor(60, 255, 150); break;
        case EGeoStreamState::Connecting: State = TEXT("LIVE LINK // CONNECTING"); StateColor = FColor(40, 210, 255); break;
        case EGeoStreamState::Degraded: State = TEXT("LIVE LINK // DEGRADED"); StateColor = FColor(255, 160, 30); break;
        default: State = TEXT("LIVE LINK // OFFLINE"); break;
        }
    }
    StreamText->SetText(FText::FromString(State)); StreamText->SetTextRenderColor(StateColor);
    if (Data)
    {
        const FGeoRuntimeStatistics Stats = Data->GetStatistics();
        TelemetryText->SetText(FText::FromString(FString::Printf(TEXT("ACTIVE %06d  //  RX %09lld  //  DROP %04lld  //  EVICT %07lld"), Stats.ActiveMessages, Stats.AcceptedMessages, Stats.DroppedMessages, Stats.EvictedMessages)));
    }
    if (Selection && Selection->HasSelection())
    {
        const FGeoMessageEnvelope Message = Selection->GetSelection();
        const FString Title = Message.Properties.FindRef(TEXT("display.title")).IsEmpty() ? Message.SemanticType : Message.Properties.FindRef(TEXT("display.title"));
        const FString From = Message.Properties.FindRef(TEXT("display.from")).IsEmpty() ? Message.FromEntityId : Message.Properties.FindRef(TEXT("display.from"));
        const FString To = Message.Properties.FindRef(TEXT("display.to")).IsEmpty() ? Message.ToEntityId : Message.Properties.FindRef(TEXT("display.to"));
        FString Analysis;
        if (Message.Geometry.Positions.Num() >= 2)
        {
            const FGeoPosition& Start = Message.Geometry.Positions[0];
            const FGeoPosition& End = Message.Geometry.Positions.Last();
            const double DistanceKm = UGeoMathLibrary::GreatCircleDistanceKm(Start, End);
            const double Bearing = UGeoMathLibrary::InitialBearingDegrees(Start, End);
            const double GraylineKm = FMath::Min(UGeoMathLibrary::GraylineDistanceKm(Start, Message.Time.ObservedUtc), UGeoMathLibrary::GraylineDistanceKm(End, Message.Time.ObservedUtc));
            Analysis = FString::Printf(TEXT("DISTANCE %.0f KM   AZIMUTH %03.0f DEG   GRAYLINE %.0f KM"), DistanceKm, Bearing, GraylineKm);
        }
        const FString Primary = Message.Properties.FindRef(TEXT("display.primary"));
        const FString Secondary = Message.Properties.FindRef(TEXT("display.secondary"));
        SelectionText->SetText(FText::FromString(FString::Printf(TEXT("%s\n%s  >  %s\n%s\n%s\n%s\n%s UTC  //  ESC TO RELEASE"), *Title.ToUpper(), *From, *To, *Primary, *Secondary, *Analysis, *Message.Time.ObservedUtc.ToIso8601())));
        SelectionText->SetTextRenderColor(FColor(225, 255, 255));
    }
    else
    {
        const FString TimelineState = Timeline && Timeline->IsPaused() ? TEXT("PAUSED") : (Timeline && !Timeline->IsLive() ? TEXT("REPLAY") : TEXT("LIVE"));
        SelectionText->SetText(FText::FromString(FString::Printf(TEXT("%s // SELECT PATH WITH LMB\nORBIT // RMB   ZOOM // WHEEL\nPAUSE // SPACE   RETURN LIVE // L   HUD // TAB\nPATHS // V   HEATMAP // H   IONOSPHERE // I   MODE // N"), *TimelineState)));
        SelectionText->SetTextRenderColor(Timeline && Timeline->IsPaused() ? FColor(255, 180, 50) : FColor(110, 225, 255));
    }
}
