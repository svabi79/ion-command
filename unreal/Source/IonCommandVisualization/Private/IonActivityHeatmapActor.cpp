#include "IonActivityHeatmapActor.h"

#include "Components/InstancedStaticMeshComponent.h"
#include "Components/SceneComponent.h"
#include "GeoDataSubsystem.h"
#include "GeoMathLibrary.h"
#include "UObject/ConstructorHelpers.h"

AIonActivityHeatmapActor::AIonActivityHeatmapActor()
{
    PrimaryActorTick.bCanEverTick = true;
    PrimaryActorTick.TickInterval = 1.0f;
    SceneRoot = CreateDefaultSubobject<USceneComponent>(TEXT("Root"));
    SetRootComponent(SceneRoot);
    static ConstructorHelpers::FObjectFinder<UStaticMesh> PlaneMesh(TEXT("/Engine/BasicShapes/Plane.Plane"));
    Splats = CreateDefaultSubobject<UInstancedStaticMeshComponent>(TEXT("HeatSplats"));
    Splats->SetupAttachment(SceneRoot);
    Splats->SetStaticMesh(PlaneMesh.Object);
    Splats->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    Splats->SetCastShadow(false);
    Splats->SetNumCustomDataFloats(1);
    if (UMaterialInterface* HeatMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/M_ActivityHeat.M_ActivityHeat")))
    {
        Splats->SetMaterial(0, HeatMaterial);
    }
    Heat.SetNumZeroed(LonCells * LatCells);
}

void AIonActivityHeatmapActor::BeginPlay()
{
    Super::BeginPlay();
    SetActorHiddenInGame(true);
    if (UGameInstance* GameInstance = GetGameInstance())
    {
        if (UGeoDataSubsystem* Data = GameInstance->GetSubsystem<UGeoDataSubsystem>())
        {
            DataSubsystem = Data;
            Data->OnMessageAccepted().AddUObject(this, &AIonActivityHeatmapActor::OnMessageAccepted);
            Data->OnDataReset().AddUObject(this, &AIonActivityHeatmapActor::OnDataReset);
            for (const FGeoMessageEnvelope& Message : Data->GetActiveMessages()) OnMessageAccepted(Message);
        }
    }
}

void AIonActivityHeatmapActor::EndPlay(const EEndPlayReason::Type EndPlayReason)
{
    if (UGeoDataSubsystem* Data = DataSubsystem.Get())
    {
        Data->OnMessageAccepted().RemoveAll(this);
        Data->OnDataReset().RemoveAll(this);
    }
    Super::EndPlay(EndPlayReason);
}

void AIonActivityHeatmapActor::OnMessageAccepted(const FGeoMessageEnvelope& Message)
{
    const bool bPathTraffic = (Message.Geometry.Type == EGeoGeometryType::GreatCircle || Message.Geometry.Type == EGeoGeometryType::Arc) && Message.Geometry.Positions.Num() >= 2;
    if (!bPathTraffic) return;
    AccumulatePosition(Message.Geometry.Positions[0]);
    AccumulatePosition(Message.Geometry.Positions.Last());
}

void AIonActivityHeatmapActor::OnDataReset()
{
    FMemory::Memzero(Heat.GetData(), Heat.Num() * sizeof(float));
}

void AIonActivityHeatmapActor::AccumulatePosition(const FGeoPosition& Position)
{
    const int32 LonCell = FMath::Clamp(static_cast<int32>((Position.Longitude + 180.0) / 360.0 * LonCells), 0, LonCells - 1);
    const int32 LatCell = FMath::Clamp(static_cast<int32>((Position.Latitude + 90.0) / 180.0 * LatCells), 0, LatCells - 1);
    Heat[LatCell * LonCells + LonCell] += 1.0f;
}

void AIonActivityHeatmapActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    const float Decay = static_cast<float>(FMath::Pow(DecayPerSecond, DeltaSeconds));
    for (float& Cell : Heat) Cell *= Decay;
    if (IsHidden()) return;
    RebuildSplats();
}

void AIonActivityHeatmapActor::RebuildSplats()
{
    float MaxHeat = 0.0f;
    for (const float Cell : Heat) MaxHeat = FMath::Max(MaxHeat, Cell);
    Splats->ClearInstances();
    if (MaxHeat < 1.0f) return;

    struct FSplat { int32 Cell; float Value; };
    TArray<FSplat> Ranked;
    Ranked.Reserve(512);
    for (int32 Cell = 0; Cell < Heat.Num(); ++Cell)
    {
        if (Heat[Cell] > MaxHeat * 0.02f) Ranked.Add({Cell, Heat[Cell]});
    }
    if (Ranked.Num() > MaxSplats)
    {
        Ranked.Sort([](const FSplat& A, const FSplat& B) { return A.Value > B.Value; });
        Ranked.SetNum(MaxSplats);
    }

    // 5-degree cell edge on the surface, widened for overlap so neighbouring
    // splats blend into a continuous field.
    const double CellArcUnits = GlobeRadius * PI / LatCells;
    TArray<FTransform> Transforms;
    Transforms.Reserve(Ranked.Num());
    TArray<float> Values;
    Values.Reserve(Ranked.Num());
    for (const FSplat& Splat : Ranked)
    {
        const int32 LatCell = Splat.Cell / LonCells;
        const int32 LonCell = Splat.Cell % LonCells;
        const double Latitude = (LatCell + 0.5) / LatCells * 180.0 - 90.0;
        if (FMath::Abs(Latitude) > 85.0) continue;
        const double Longitude = (LonCell + 0.5) / LonCells * 360.0 - 180.0;
        const FVector Normal = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Latitude, Longitude);
        const FVector Location = Normal * (GlobeRadius + SurfaceOffset);
        const double Scale = CellArcUnits / 100.0 * 1.9;
        const double LonScale = FMath::Max(Scale * FMath::Cos(FMath::DegreesToRadians(Latitude)), Scale * 0.25);
        FTransform Transform(FRotationMatrix::MakeFromZ(Normal).ToQuat(), Location, FVector(LonScale, Scale, 1.0));
        Transforms.Add(Transform);
        // Square-root contrast keeps mid-heat cells visible next to hotspots.
        Values.Add(FMath::Sqrt(Splat.Value / MaxHeat));
    }
    Splats->AddInstances(Transforms, false);
    for (int32 Index = 0; Index < Values.Num(); ++Index)
    {
        Splats->SetCustomDataValue(Index, 0, Values[Index], false);
    }
    Splats->MarkRenderStateDirty();
}
