#include "GeoPathLayerActor.h"

#include "Components/InstancedStaticMeshComponent.h"
#include "Components/SceneComponent.h"
#include "GeoDataSubsystem.h"
#include "GeoLayerSubsystem.h"
#include "GeoMathLibrary.h"
#include "GeoSelectionSubsystem.h"
#include "Materials/MaterialInstanceDynamic.h"
#include "UObject/ConstructorHelpers.h"

namespace
{
constexpr double PathHeight = 4.0;
constexpr double MaxEdgeDegrees = 2.8;
constexpr int32 LegendClassCount = 5;

const FLinearColor LegendColors[LegendClassCount] = {
    FLinearColor(1.00f, 0.72f, 0.18f),
    FLinearColor(0.25f, 0.70f, 0.62f),
    FLinearColor(0.20f, 0.85f, 1.00f),
    FLinearColor(0.22f, 0.45f, 0.95f),
    FLinearColor(0.92f, 0.28f, 0.72f),
};

FLinearColor ParseColorProperty(const FString& Text, const FLinearColor& Fallback)
{
    TArray<FString> Parts;
    Text.ParseIntoArray(Parts, TEXT(","), true);
    if (Parts.Num() < 3)
    {
        return Fallback;
    }
    return FLinearColor(FCString::Atof(*Parts[0]), FCString::Atof(*Parts[1]), FCString::Atof(*Parts[2]), 1.0f);
}
}

AGeoPathLayerActor::AGeoPathLayerActor()
{
    PrimaryActorTick.bCanEverTick = true;
    SceneRoot = CreateDefaultSubobject<USceneComponent>(TEXT("Root"));
    SetRootComponent(SceneRoot);

    static ConstructorHelpers::FObjectFinder<UStaticMesh> SegmentMesh(TEXT("/Engine/BasicShapes/Cube.Cube"));
    const TCHAR* ClassNames[] = {TEXT("PathClass0"), TEXT("PathClass1"), TEXT("PathClass2"), TEXT("PathClass3"), TEXT("PathClass4")};
    ClassMeshes.Reserve(LegendClassCount);
    for (int32 Index = 0; Index < LegendClassCount; ++Index)
    {
        UInstancedStaticMeshComponent* Mesh = CreateDefaultSubobject<UInstancedStaticMeshComponent>(ClassNames[Index]);
        Mesh->SetupAttachment(SceneRoot);
        Mesh->SetStaticMesh(SegmentMesh.Object);
        Mesh->SetCollisionEnabled(ECollisionEnabled::NoCollision);
        Mesh->SetCastShadow(false);
        if (UMaterialInterface* PathMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Track.MI_Track")))
        {
            Mesh->SetMaterial(0, PathMaterial);
        }
        ClassMeshes.Add(Mesh);
    }

    SelectionMesh = CreateDefaultSubobject<UInstancedStaticMeshComponent>(TEXT("SelectedPath"));
    SelectionMesh->SetupAttachment(SceneRoot);
    SelectionMesh->SetStaticMesh(SegmentMesh.Object);
    SelectionMesh->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    SelectionMesh->SetCastShadow(false);
    if (UMaterialInterface* SelectionMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Signal_Selected.MI_Signal_Selected")))
    {
        SelectionMesh->SetMaterial(0, SelectionMaterial);
    }
}

void AGeoPathLayerActor::OnConstruction(const FTransform& Transform)
{
    Super::OnConstruction(Transform);
    if (GetWorld() && !GetWorld()->IsGameWorld())
    {
        BuildEditorPreview();
    }
}

void AGeoPathLayerActor::PostRegisterAllComponents()
{
    Super::PostRegisterAllComponents();
    if (GetWorld() && !GetWorld()->IsGameWorld())
    {
        BuildEditorPreview();
    }
}

void AGeoPathLayerActor::BuildEditorPreview()
{
    Reset();
    constexpr int32 PreviewCount = 5;
    for (int32 Index = 0; Index < PreviewCount; ++Index)
    {
        FGeoMessageEnvelope Preview;
        Preview.MessageId = FString::Printf(TEXT("preview-path-%d"), Index);
        Preview.EntityId = Preview.MessageId;
        Preview.MessageType = EGeoMessageType::Relationship;
        Preview.SemanticType = TEXT("preview.path");
        Preview.Geometry.Type = EGeoGeometryType::LineString;
        const double Lon = -80.0 + Index * 40.0;
        const double Lat = -8.0 + (Index % 2) * 16.0;
        FGeoPosition A; A.Longitude = Lon; A.Latitude = Lat;
        FGeoPosition B; B.Longitude = Lon + 18.0; B.Latitude = Lat + 12.0;
        FGeoPosition C; C.Longitude = Lon + 32.0; C.Latitude = Lat + 2.0;
        Preview.Geometry.Positions.Add(A);
        Preview.Geometry.Positions.Add(B);
        Preview.Geometry.Positions.Add(C);
        Preview.Properties.Add(TEXT("visual.legendIndex"), FString::FromInt(Index % LegendClassCount));
        Preview.Properties.Add(TEXT("visual.color"), FString::Printf(TEXT("%f,%f,%f"), LegendColors[Index % LegendClassCount].R, LegendColors[Index % LegendClassCount].G, LegendColors[Index % LegendClassCount].B));
        Submit(Preview);
    }
    RebuildMeshes();
}

void AGeoPathLayerActor::BeginPlay()
{
    Super::BeginPlay();
    Reset();
    ClassMaterials.Reset();
    for (int32 Index = 0; Index < ClassMeshes.Num(); ++Index)
    {
        UMaterialInstanceDynamic* Material = ClassMeshes[Index] ? ClassMeshes[Index]->CreateAndSetMaterialInstanceDynamic(0) : nullptr;
        if (Material)
        {
            Material->SetVectorParameterValue(TEXT("Color"), LegendColors[Index]);
            Material->SetScalarParameterValue(TEXT("Intensity"), 3.6f);
        }
        ClassMaterials.Add(Material);
    }
    if (UMaterialInstanceDynamic* Selection = SelectionMesh->CreateAndSetMaterialInstanceDynamic(0))
    {
        Selection->SetVectorParameterValue(TEXT("Color"), FLinearColor(0.82f, 1.0f, 1.0f));
    }
    if (UGameInstance* GameInstance = GetGameInstance())
    {
        DataSubsystem = GameInstance->GetSubsystem<UGeoDataSubsystem>();
        if (DataSubsystem.IsValid())
        {
            DataSubsystem->OnMessageAccepted().AddUObject(this, &AGeoPathLayerActor::OnMessageAccepted);
            DataSubsystem->OnDataReset().AddUObject(this, &AGeoPathLayerActor::Reset);
            for (const FGeoMessageEnvelope& Message : DataSubsystem->GetActiveMessages())
            {
                OnMessageAccepted(Message);
            }
        }
        if (UGeoLayerSubsystem* LayerSubsystem = GameInstance->GetSubsystem<UGeoLayerSubsystem>())
        {
            const FGeoLayerManifest Manifest = CreateLayerManifest();
            LayerSubsystem->RegisterLayer(Manifest);
            LayerSubsystem->OnLayerVisibilityChanged().AddUObject(this, &AGeoPathLayerActor::OnLayerVisibilityChanged);
            SetActorHiddenInGame(!LayerSubsystem->IsLayerVisible(Manifest.LayerId));
        }
    }
}

void AGeoPathLayerActor::EndPlay(const EEndPlayReason::Type EndPlayReason)
{
    if (DataSubsystem.IsValid())
    {
        DataSubsystem->OnMessageAccepted().RemoveAll(this);
        DataSubsystem->OnDataReset().RemoveAll(this);
    }
    if (UGameInstance* GameInstance = GetGameInstance())
    {
        if (UGeoLayerSubsystem* LayerSubsystem = GameInstance->GetSubsystem<UGeoLayerSubsystem>())
        {
            LayerSubsystem->OnLayerVisibilityChanged().RemoveAll(this);
        }
    }
    Super::EndPlay(EndPlayReason);
}

void AGeoPathLayerActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    RefreshSelectionHighlight();
    const double Now = FPlatformTime::Seconds();
    if (Now - LastExpiryCheck < 3.0 && !bNeedsRebuild)
    {
        return;
    }
    LastExpiryCheck = Now;
    const double RenderNow = GetWorld() ? GetWorld()->GetTimeSeconds() : 0.0;
    EvictExpiredPaths(RenderNow, FMath::Max(8, MaxVisiblePaths / 8));
    if (bNeedsRebuild)
    {
        RebuildMeshes();
    }
}

bool AGeoPathLayerActor::Supports(const FGeoMessageEnvelope& Message) const
{
    return (Message.Geometry.Type == EGeoGeometryType::LineString || Message.Geometry.Type == EGeoGeometryType::MultiLineString)
        && Message.Geometry.Positions.Num() >= 2;
}

void AGeoPathLayerActor::Submit(const FGeoMessageEnvelope& Message)
{
    if (!Supports(Message))
    {
        return;
    }
    const FString Key = ResolveEntityKey(Message);
    const double NowSeconds = GetWorld() ? GetWorld()->GetTimeSeconds() : 0.0;
    FRenderedGeoPath Path;
    Path.EntityKey = Key;
    Path.Message = Message;
    Path.Color = ResolveColor(Message);
    Path.LegendIndex = ResolveLegendIndex(Message);
    Path.LastSeenSeconds = NowSeconds;
    Path.ExpireAtSeconds = 0.0;
    if (Message.Time.bHasValidUntil)
    {
        const FTimespan Remaining = Message.Time.ValidUntilUtc - FDateTime::UtcNow();
        Path.ExpireAtSeconds = NowSeconds + FMath::Max(30.0, Remaining.GetTotalSeconds());
    }
    if (const int32* Existing = EntityToPath.Find(Key))
    {
        ActivePaths[*Existing] = Path;
        ++RuntimeStats.IncrementalUpdates;
        bNeedsRebuild = true;
        return;
    }
    if (ActivePaths.Num() >= MaxVisiblePaths)
    {
        TrimToCapacity(FMath::Max(1, MaxVisiblePaths / 8));
    }
    const int32 Index = ActivePaths.Add(Path);
    EntityToPath.Add(Key, Index);
    ++RuntimeStats.IncrementalInserts;
    bNeedsRebuild = true;
}

void AGeoPathLayerActor::Reset()
{
    ActivePaths.Reset();
    EntityToPath.Reset();
    HighlightedMessageId.Reset();
    bNeedsRebuild = false;
    for (UInstancedStaticMeshComponent* Mesh : ClassMeshes)
    {
        if (Mesh)
        {
            Mesh->ClearInstances();
        }
    }
    if (SelectionMesh)
    {
        SelectionMesh->ClearInstances();
    }
}

void AGeoPathLayerActor::OnMessageAccepted(const FGeoMessageEnvelope& Message)
{
    Submit(Message);
}

FString AGeoPathLayerActor::ResolveEntityKey(const FGeoMessageEnvelope& Message) const
{
    if (!Message.EntityId.IsEmpty())
    {
        return Message.SemanticType + TEXT("|") + Message.EntityId;
    }
    return Message.MessageId;
}

FLinearColor AGeoPathLayerActor::ResolveColor(const FGeoMessageEnvelope& Message) const
{
    const FString Explicit = Message.Properties.FindRef(TEXT("visual.color"));
    if (!Explicit.IsEmpty())
    {
        return ParseColorProperty(Explicit, LegendColors[2]);
    }
    return LegendColors[FMath::Clamp(ResolveLegendIndex(Message), 0, LegendClassCount - 1)];
}

int32 AGeoPathLayerActor::ResolveLegendIndex(const FGeoMessageEnvelope& Message) const
{
    const FString Text = Message.Properties.FindRef(TEXT("visual.legendIndex"));
    if (!Text.IsEmpty())
    {
        return FMath::Clamp(FCString::Atoi(*Text), 0, LegendClassCount - 1);
    }
    return 2;
}

FString AGeoPathLayerActor::GetLegendNote() const
{
    return TEXT("planned amber · short teal\nregional cyan · ocean blue · trunk magenta");
}

bool AGeoPathLayerActor::IsExpired(const FRenderedGeoPath& Path, double NowSeconds) const
{
    if (Path.ExpireAtSeconds > 0.0)
    {
        return NowSeconds > Path.ExpireAtSeconds;
    }
    return NowSeconds - Path.LastSeenSeconds > PathLifetimeSeconds;
}

void AGeoPathLayerActor::RemovePathAt(int32 Index)
{
    if (!ActivePaths.IsValidIndex(Index))
    {
        return;
    }
    const int32 Last = ActivePaths.Num() - 1;
    EntityToPath.Remove(ActivePaths[Index].EntityKey);
    if (Index != Last)
    {
        ActivePaths[Index] = ActivePaths[Last];
        EntityToPath.Add(ActivePaths[Index].EntityKey, Index);
    }
    ActivePaths.RemoveAt(Last, 1, EAllowShrinking::No);
    ++RuntimeStats.IncrementalRemovals;
    bNeedsRebuild = true;
}

int32 AGeoPathLayerActor::EvictExpiredPaths(double NowSeconds, int32 MaxRemovals)
{
    int32 Removed = 0;
    int32 Index = 0;
    while (Index < ActivePaths.Num() && Removed < MaxRemovals)
    {
        if (IsExpired(ActivePaths[Index], NowSeconds))
        {
            RemovePathAt(Index);
            ++RuntimeStats.ExpiredRemovals;
            ++Removed;
            continue;
        }
        ++Index;
    }
    return Removed;
}

int32 AGeoPathLayerActor::TrimToCapacity(int32 MaxRemovals)
{
    int32 Removed = 0;
    while (ActivePaths.Num() >= MaxVisiblePaths && Removed < MaxRemovals)
    {
        int32 Oldest = 0;
        for (int32 Index = 1; Index < ActivePaths.Num(); ++Index)
        {
            if (ActivePaths[Index].LastSeenSeconds < ActivePaths[Oldest].LastSeenSeconds)
            {
                Oldest = Index;
            }
        }
        RemovePathAt(Oldest);
        ++RuntimeStats.CapacityEvictions;
        ++Removed;
    }
    return Removed;
}

void AGeoPathLayerActor::ProjectLine(const TArray<FGeoPosition>& Line, TArray<FVector>& OutWorld) const
{
    OutWorld.Reset();
    if (Line.Num() < 2)
    {
        return;
    }
    TArray<FGeoPosition> Work = Line;
    if (Work.Num() > MaxVerticesPerLine)
    {
        TArray<FGeoPosition> Reduced;
        const double Step = static_cast<double>(Work.Num() - 1) / static_cast<double>(MaxVerticesPerLine - 1);
        Reduced.Reserve(MaxVerticesPerLine);
        for (int32 Index = 0; Index < MaxVerticesPerLine - 1; ++Index)
        {
            Reduced.Add(Work[FMath::Clamp(static_cast<int32>(Index * Step), 0, Work.Num() - 1)]);
        }
        Reduced.Add(Work.Last());
        Work = MoveTemp(Reduced);
    }

    TArray<FGeoPosition> Dense;
    Dense.Reserve(Work.Num() * 2);
    for (int32 Index = 0; Index < Work.Num() - 1; ++Index)
    {
        const FGeoPosition& From = Work[Index];
        const FGeoPosition& To = Work[Index + 1];
        Dense.Add(From);
        const double Distance = UGeoMathLibrary::GreatCircleDistanceKm(From, To);
        const int32 Splits = FMath::Clamp(static_cast<int32>(Distance / (MaxEdgeDegrees * 111.0)), 0, 8);
        for (int32 Split = 1; Split <= Splits; ++Split)
        {
            Dense.Add(UGeoMathLibrary::GreatCircleInterpolation(From, To, static_cast<double>(Split) / (Splits + 1)));
        }
    }
    Dense.Add(Work.Last());

    const double Radius = GlobeRadius + PathHeight;
    OutWorld.Reserve(Dense.Num());
    for (const FGeoPosition& Position : Dense)
    {
        OutWorld.Add(GetActorTransform().TransformPosition(
            UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Position.Latitude, Position.Longitude) * Radius));
    }
}

void AGeoPathLayerActor::AppendSegmentTransforms(TArray<FTransform>& Out, const TArray<FVector>& WorldLine) const
{
    if (WorldLine.Num() < 2)
    {
        return;
    }
    for (int32 Index = 0; Index < WorldLine.Num() - 1; ++Index)
    {
        const FVector& Previous = WorldLine[Index];
        const FVector& Current = WorldLine[Index + 1];
        const FVector Delta = Current - Previous;
        const double Length = Delta.Size();
        if (Length < 0.05)
        {
            continue;
        }
        const FVector Midpoint = (Previous + Current) * 0.5;
        const FRotator Rotation = FRotationMatrix::MakeFromZ(Delta.GetSafeNormal()).Rotator();
        Out.Emplace(Rotation, Midpoint, FVector(PathThickness, PathThickness, Length / 100.0));
    }
}

void AGeoPathLayerActor::RebuildMeshes()
{
    bNeedsRebuild = false;
    TArray<TArray<FTransform>> Transforms;
    Transforms.SetNum(ClassMeshes.Num());
    for (TArray<FTransform>& Bucket : Transforms)
    {
        Bucket.Reserve(256);
    }
    for (const FRenderedGeoPath& Path : ActivePaths)
    {
        const int32 ClassIndex = FMath::Clamp(Path.LegendIndex, 0, Transforms.Num() - 1);
        const int32 Lines = Path.Message.Geometry.NumLines();
        for (int32 LineIndex = 0; LineIndex < Lines; ++LineIndex)
        {
            TArray<FGeoPosition> Line;
            if (!Path.Message.Geometry.GetLine(LineIndex, Line))
            {
                continue;
            }
            TArray<FVector> World;
            ProjectLine(Line, World);
            AppendSegmentTransforms(Transforms[ClassIndex], World);
        }
    }
    for (int32 Index = 0; Index < ClassMeshes.Num(); ++Index)
    {
        UInstancedStaticMeshComponent* Mesh = ClassMeshes[Index];
        if (!Mesh)
        {
            continue;
        }
        Mesh->ClearInstances();
        if (Transforms[Index].Num() > 0)
        {
            Mesh->AddInstances(Transforms[Index], false, true);
        }
    }
}

void AGeoPathLayerActor::RefreshSelectionHighlight()
{
    const UGeoSelectionSubsystem* Selection = GetGameInstance() ? GetGameInstance()->GetSubsystem<UGeoSelectionSubsystem>() : nullptr;
    const FGeoMessageEnvelope Selected = Selection && Selection->HasSelection() ? Selection->GetSelection() : FGeoMessageEnvelope();
    const FString NextMessageId = Supports(Selected) ? Selected.MessageId : FString();
    if (NextMessageId == HighlightedMessageId)
    {
        return;
    }
    HighlightedMessageId = NextMessageId;
    SelectionMesh->ClearInstances();
    if (HighlightedMessageId.IsEmpty())
    {
        return;
    }
    TArray<FTransform> Transforms;
    const int32 Lines = Selected.Geometry.NumLines();
    for (int32 LineIndex = 0; LineIndex < Lines; ++LineIndex)
    {
        TArray<FGeoPosition> Line;
        if (!Selected.Geometry.GetLine(LineIndex, Line))
        {
            continue;
        }
        TArray<FVector> World;
        ProjectLine(Line, World);
        AppendSegmentTransforms(Transforms, World);
    }
    if (Transforms.Num() > 0)
    {
        SelectionMesh->AddInstances(Transforms, true, true);
    }
}

bool AGeoPathLayerActor::FindClosestMessageToRay(const FVector& RayOrigin, const FVector& RayDirection, double RayLength, double MaxDistance, FGeoMessageEnvelope& OutMessage) const
{
    const FVector RayEnd = RayOrigin + RayDirection.GetSafeNormal() * RayLength;
    double BestDistanceSquared = FMath::Square(MaxDistance);
    bool bFound = false;
    for (const FRenderedGeoPath& Path : ActivePaths)
    {
        const int32 Lines = Path.Message.Geometry.NumLines();
        for (int32 LineIndex = 0; LineIndex < Lines; ++LineIndex)
        {
            TArray<FGeoPosition> Line;
            if (!Path.Message.Geometry.GetLine(LineIndex, Line))
            {
                continue;
            }
            TArray<FVector> World;
            ProjectLine(Line, World);
            for (int32 Index = 0; Index < World.Num() - 1; ++Index)
            {
                FVector ClosestRay;
                FVector ClosestEdge;
                FMath::SegmentDistToSegmentSafe(RayOrigin, RayEnd, World[Index], World[Index + 1], ClosestRay, ClosestEdge);
                const double DistanceSquared = FVector::DistSquared(ClosestRay, ClosestEdge);
                if (DistanceSquared < BestDistanceSquared)
                {
                    BestDistanceSquared = DistanceSquared;
                    OutMessage = Path.Message;
                    bFound = true;
                }
            }
        }
    }
    return bFound;
}

void AGeoPathLayerActor::OnLayerVisibilityChanged(const FString& LayerId, bool bVisible)
{
    if (LayerId == TEXT("core.paths"))
    {
        SetActorHiddenInGame(!bVisible);
    }
}

FGeoLayerManifest AGeoPathLayerActor::CreateLayerManifest() const
{
    FGeoLayerManifest Manifest;
    Manifest.LayerId = TEXT("core.paths");
    Manifest.DisplayName = TEXT("Geospatial Paths");
    Manifest.GeometryTypes = {EGeoGeometryType::LineString, EGeoGeometryType::MultiLineString};
    Manifest.bSupportsAggregation = false;
    return Manifest;
}

FGeoRenderLayerStatistics AGeoPathLayerActor::GetRenderStatistics() const
{
    FGeoRenderLayerStatistics Stats = RuntimeStats;
    Stats.TrackedItems = ActivePaths.Num();
    int32 Instances = 0;
    for (const UInstancedStaticMeshComponent* Mesh : ClassMeshes)
    {
        if (Mesh)
        {
            Instances += Mesh->GetInstanceCount();
        }
    }
    Stats.RenderedInstances = Instances;
    return Stats;
}
