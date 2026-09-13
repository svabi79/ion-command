#include "GeoAreaLayerActor.h"

#include "Algo/Reverse.h"
#include "Components/InstancedStaticMeshComponent.h"
#include "Components/SceneComponent.h"
#include "GeoDataSubsystem.h"
#include "GeoLayerSubsystem.h"
#include "GeoMathLibrary.h"
#include "GeoCoveragePinSubsystem.h"
#include "GeoSelectionSubsystem.h"
#include "Materials/MaterialInstanceDynamic.h"
#include "ProceduralMeshComponent.h"
#include "UObject/ConstructorHelpers.h"

namespace
{
constexpr double FillHeight = 3.5;
constexpr double OutlineHeight = 5.5;
constexpr double MaxEdgeDegrees = 2.8;

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

double UnwrapDelta(double Previous, double Current)
{
    double Value = Current;
    while (Value - Previous > 180.0) Value -= 360.0;
    while (Value - Previous < -180.0) Value += 360.0;
    return Value;
}

double SignedArea(const TArray<FGeoPosition>& Ring)
{
    double Area = 0.0;
    const int32 Count = Ring.Num();
    for (int32 Index = 0; Index < Count; ++Index)
    {
        const FGeoPosition& A = Ring[Index];
        const FGeoPosition& B = Ring[(Index + 1) % Count];
        Area += (B.Longitude - A.Longitude) * (B.Latitude + A.Latitude);
    }
    return Area;
}

bool PointInTriangle(const FGeoPosition& P, const FGeoPosition& A, const FGeoPosition& B, const FGeoPosition& C)
{
    const double D1 = (P.Longitude - B.Longitude) * (A.Latitude - B.Latitude) - (A.Longitude - B.Longitude) * (P.Latitude - B.Latitude);
    const double D2 = (P.Longitude - C.Longitude) * (B.Latitude - C.Latitude) - (B.Longitude - C.Longitude) * (P.Latitude - C.Latitude);
    const double D3 = (P.Longitude - A.Longitude) * (C.Latitude - A.Latitude) - (C.Longitude - A.Longitude) * (P.Latitude - A.Latitude);
    const bool bHasNeg = (D1 < 0.0) || (D2 < 0.0) || (D3 < 0.0);
    const bool bHasPos = (D1 > 0.0) || (D2 > 0.0) || (D3 > 0.0);
    return !(bHasNeg && bHasPos);
}

void CloseAndDedup(TArray<FGeoPosition>& Ring)
{
    if (Ring.Num() >= 2)
    {
        const FGeoPosition& First = Ring[0];
        const FGeoPosition& Last = Ring.Last();
        if (FMath::IsNearlyEqual(First.Longitude, Last.Longitude, 1e-6) && FMath::IsNearlyEqual(First.Latitude, Last.Latitude, 1e-6))
        {
            Ring.Pop(EAllowShrinking::No);
        }
    }
}
}

AGeoAreaLayerActor::AGeoAreaLayerActor()
{
    PrimaryActorTick.bCanEverTick = true;
    SceneRoot = CreateDefaultSubobject<USceneComponent>(TEXT("Root"));
    SetRootComponent(SceneRoot);

    FillMesh = CreateDefaultSubobject<UProceduralMeshComponent>(TEXT("AreaFill"));
    FillMesh->SetupAttachment(SceneRoot);
    FillMesh->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    FillMesh->SetCastShadow(false);
    FillMesh->bUseAsyncCooking = false;

    static ConstructorHelpers::FObjectFinder<UStaticMesh> SegmentMesh(TEXT("/Engine/BasicShapes/Cube.Cube"));
    OutlineMesh = CreateDefaultSubobject<UInstancedStaticMeshComponent>(TEXT("AreaOutline"));
    OutlineMesh->SetupAttachment(SceneRoot);
    OutlineMesh->SetStaticMesh(SegmentMesh.Object);
    OutlineMesh->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    OutlineMesh->SetCastShadow(false);
    if (UMaterialInterface* OutlineMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Track.MI_Track")))
    {
        OutlineMesh->SetMaterial(0, OutlineMaterial);
    }

    SelectionMesh = CreateDefaultSubobject<UInstancedStaticMeshComponent>(TEXT("SelectedAreaOutline"));
    SelectionMesh->SetupAttachment(SceneRoot);
    SelectionMesh->SetStaticMesh(SegmentMesh.Object);
    SelectionMesh->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    SelectionMesh->SetCastShadow(false);
    if (UMaterialInterface* SelectionMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Signal_Selected.MI_Signal_Selected")))
    {
        SelectionMesh->SetMaterial(0, SelectionMaterial);
    }
}

void AGeoAreaLayerActor::OnConstruction(const FTransform& Transform)
{
    Super::OnConstruction(Transform);
    if (GetWorld() && !GetWorld()->IsGameWorld())
    {
        BuildEditorPreview();
    }
}

void AGeoAreaLayerActor::PostRegisterAllComponents()
{
    Super::PostRegisterAllComponents();
    if (GetWorld() && !GetWorld()->IsGameWorld())
    {
        BuildEditorPreview();
    }
}

void AGeoAreaLayerActor::BuildEditorPreview()
{
    Reset();
    constexpr int32 PreviewCount = 4;
    for (int32 Index = 0; Index < PreviewCount; ++Index)
    {
        FGeoMessageEnvelope Preview;
        Preview.MessageId = FString::Printf(TEXT("preview-area-%d"), Index);
        Preview.EntityId = Preview.MessageId;
        Preview.MessageType = EGeoMessageType::Area;
        Preview.SemanticType = TEXT("preview.area");
        Preview.Geometry.Type = EGeoGeometryType::Polygon;
        const double Lon = -40.0 + Index * 55.0;
        const double Lat = 8.0 + (Index % 2) * 12.0;
        const double Half = 8.0 + Index * 1.5;
        const double CornerLon[] = {Lon - Half, Lon + Half, Lon + Half * 0.7, Lon - Half * 0.8, Lon - Half};
        const double CornerLat[] = {Lat - Half * 0.6, Lat - Half * 0.45, Lat + Half, Lat + Half * 0.7, Lat - Half * 0.6};
        for (int32 Corner = 0; Corner < 5; ++Corner)
        {
            FGeoPosition Position;
            Position.Longitude = CornerLon[Corner];
            Position.Latitude = CornerLat[Corner];
            Preview.Geometry.Positions.Add(Position);
        }
        Preview.Geometry.RingLengths.Add(5);
        Preview.Properties.Add(TEXT("visual.color"), TEXT("0.15,0.85,1.0"));
        Submit(Preview);
    }
    RebuildMeshes();
}

void AGeoAreaLayerActor::BeginPlay()
{
    Super::BeginPlay();
    Reset();
    UMaterialInterface* FillParent = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_AreaFill.MI_AreaFill"));
    if (!FillParent)
    {
        FillParent = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Atmosphere.MI_Atmosphere"));
    }
    if (FillParent && FillMesh)
    {
        FillMaterial = UMaterialInstanceDynamic::Create(FillParent, this);
        FillMaterial->SetVectorParameterValue(TEXT("Color"), FLinearColor(1.0f, 0.55f, 0.12f));
        FillMaterial->SetScalarParameterValue(TEXT("Opacity"), 0.22f);
        FillMaterial->SetScalarParameterValue(TEXT("Intensity"), 1.6f);
        FillMaterial->SetScalarParameterValue(TEXT("RimExponent"), 0.08f);
        FillMesh->SetMaterial(0, FillMaterial);
    }
    if (UMaterialInstanceDynamic* Outline = OutlineMesh->CreateAndSetMaterialInstanceDynamic(0))
    {
        Outline->SetVectorParameterValue(TEXT("Color"), FLinearColor(1.0f, 0.72f, 0.22f));
        Outline->SetScalarParameterValue(TEXT("Intensity"), 4.2f);
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
            DataSubsystem->OnMessageAccepted().AddUObject(this, &AGeoAreaLayerActor::OnMessageAccepted);
            DataSubsystem->OnDataReset().AddUObject(this, &AGeoAreaLayerActor::Reset);
            for (const FGeoMessageEnvelope& Message : DataSubsystem->GetActiveMessages())
            {
                OnMessageAccepted(Message);
            }
        }
        if (UGeoLayerSubsystem* LayerSubsystem = GameInstance->GetSubsystem<UGeoLayerSubsystem>())
        {
            const FGeoLayerManifest Manifest = CreateLayerManifest();
            LayerSubsystem->RegisterLayer(Manifest);
            LayerSubsystem->OnLayerVisibilityChanged().AddUObject(this, &AGeoAreaLayerActor::OnLayerVisibilityChanged);
            SetActorHiddenInGame(!LayerSubsystem->IsLayerVisible(Manifest.LayerId));
        }
        if (UGeoCoveragePinSubsystem* Pins = GameInstance->GetSubsystem<UGeoCoveragePinSubsystem>())
        {
            Pins->OnPinsChanged.AddDynamic(this, &AGeoAreaLayerActor::OnCoveragePinsChanged);
            OnCoveragePinsChanged();
        }
    }
}

void AGeoAreaLayerActor::EndPlay(const EEndPlayReason::Type EndPlayReason)
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
        if (UGeoCoveragePinSubsystem* Pins = GameInstance->GetSubsystem<UGeoCoveragePinSubsystem>())
        {
            Pins->OnPinsChanged.RemoveAll(this);
        }
    }
    Super::EndPlay(EndPlayReason);
}

void AGeoAreaLayerActor::Tick(float DeltaSeconds)
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
    EvictExpiredAreas(RenderNow, FMath::Max(8, MaxVisibleAreas / 4));
    if (bNeedsRebuild)
    {
        RebuildMeshes();
    }
}

bool AGeoAreaLayerActor::Supports(const FGeoMessageEnvelope& Message) const
{
    return (Message.Geometry.Type == EGeoGeometryType::Polygon || Message.Geometry.Type == EGeoGeometryType::MultiPolygon)
        && Message.Geometry.Positions.Num() >= 3;
}

void AGeoAreaLayerActor::Submit(const FGeoMessageEnvelope& Message)
{
    if (!Supports(Message))
    {
        return;
    }
    if (IsCoverageGated(Message))
    {
        HoldGatedArea(Message);
        if (!EnabledCoveragePins.Contains(CoveragePinKey(Message)))
        {
            return;
        }
    }
    const FString Key = ResolveEntityKey(Message);
    const double NowSeconds = GetWorld() ? GetWorld()->GetTimeSeconds() : 0.0;
    FRenderedGeoArea Area;
    Area.EntityKey = Key;
    Area.Message = Message;
    Area.Color = ResolveColor(Message);
    Area.Opacity = ResolveOpacity(Message);
    Area.LastSeenSeconds = NowSeconds;
    Area.ExpireAtSeconds = 0.0;
    if (Message.Time.bHasValidUntil)
    {
        const FTimespan Remaining = Message.Time.ValidUntilUtc - FDateTime::UtcNow();
        Area.ExpireAtSeconds = NowSeconds + FMath::Max(30.0, Remaining.GetTotalSeconds());
    }
    if (const int32* Existing = EntityToArea.Find(Key))
    {
        ActiveAreas[*Existing] = Area;
        ++RuntimeStats.IncrementalUpdates;
        bNeedsRebuild = true;
        return;
    }
    if (ActiveAreas.Num() >= MaxVisibleAreas)
    {
        TrimToCapacity(FMath::Max(1, MaxVisibleAreas / 8));
    }
    const int32 Index = ActiveAreas.Add(Area);
    EntityToArea.Add(Key, Index);
    ++RuntimeStats.IncrementalInserts;
    bNeedsRebuild = true;
}

void AGeoAreaLayerActor::Reset()
{
    ActiveAreas.Reset();
    EntityToArea.Reset();
    HeldPinAreas.Reset();
    HighlightedMessageId.Reset();
    bNeedsRebuild = false;
    if (FillMesh)
    {
        FillMesh->ClearAllMeshSections();
    }
    if (OutlineMesh)
    {
        OutlineMesh->ClearInstances();
    }
    if (SelectionMesh)
    {
        SelectionMesh->ClearInstances();
    }
}

void AGeoAreaLayerActor::OnMessageAccepted(const FGeoMessageEnvelope& Message)
{
    Submit(Message);
}

bool AGeoAreaLayerActor::IsCoverageGated(const FGeoMessageEnvelope& Message)
{
    return Message.Properties.FindRef(TEXT("visual.defaultHidden")) == TEXT("true")
        && !CoveragePinKey(Message).IsEmpty();
}

FString AGeoAreaLayerActor::CoveragePinKey(const FGeoMessageEnvelope& Message)
{
    return Message.Properties.FindRef(TEXT("visual.pinKey"));
}

void AGeoAreaLayerActor::HoldGatedArea(const FGeoMessageEnvelope& Message)
{
    const FString Key = CoveragePinKey(Message);
    if (Key.IsEmpty())
    {
        return;
    }
    HeldPinAreas.Add(Key, Message);
    if (HeldPinAreas.Num() <= 256)
    {
        return;
    }
    TArray<FString> Extra;
    for (const TPair<FString, FGeoMessageEnvelope>& Pair : HeldPinAreas)
    {
        if (!EnabledCoveragePins.Contains(Pair.Key))
        {
            Extra.Add(Pair.Key);
        }
    }
    Extra.Sort();
    const int32 Drop = FMath::Max(1, Extra.Num() / 8);
    for (int32 Index = 0; Index < Drop && Index < Extra.Num(); ++Index)
    {
        HeldPinAreas.Remove(Extra[Index]);
    }
}

void AGeoAreaLayerActor::SetCoveragePin(const FString& PinKey, bool bEnabled)
{
    if (PinKey.IsEmpty())
    {
        return;
    }
    if (bEnabled)
    {
        EnabledCoveragePins.Add(PinKey);
        if (const FGeoMessageEnvelope* Held = HeldPinAreas.Find(PinKey))
        {
            Submit(*Held);
        }
        return;
    }
    EnabledCoveragePins.Remove(PinKey);
    DropPinnedAreas(PinKey);
}

void AGeoAreaLayerActor::DropPinnedAreas(const FString& PinKey)
{
    int32 Index = 0;
    while (Index < ActiveAreas.Num())
    {
        if (CoveragePinKey(ActiveAreas[Index].Message) == PinKey)
        {
            RemoveAreaAt(Index);
            continue;
        }
        ++Index;
    }
}

void AGeoAreaLayerActor::OnCoveragePinsChanged()
{
    const UGameInstance* GameInstance = GetGameInstance();
    const UGeoCoveragePinSubsystem* Pins = GameInstance ? GameInstance->GetSubsystem<UGeoCoveragePinSubsystem>() : nullptr;
    TSet<FString> Next;
    if (Pins)
    {
        for (const FGeoCoveragePin& Pin : Pins->GetPins())
        {
            Next.Add(Pin.Key);
        }
    }
    TArray<FString> Previous = EnabledCoveragePins.Array();
    for (const FString& Key : Previous)
    {
        if (!Next.Contains(Key))
        {
            SetCoveragePin(Key, false);
        }
    }
    for (const FString& Key : Next)
    {
        if (!EnabledCoveragePins.Contains(Key))
        {
            SetCoveragePin(Key, true);
        }
    }
}

FString AGeoAreaLayerActor::ResolveEntityKey(const FGeoMessageEnvelope& Message) const
{
    if (!Message.EntityId.IsEmpty())
    {
        return Message.SemanticType + TEXT("|") + Message.EntityId;
    }
    return Message.MessageId;
}

FLinearColor AGeoAreaLayerActor::ResolveColor(const FGeoMessageEnvelope& Message) const
{
    const FString Explicit = Message.Properties.FindRef(TEXT("visual.color"));
    if (!Explicit.IsEmpty())
    {
        return ParseColorProperty(Explicit, FLinearColor(1.0f, 0.55f, 0.12f));
    }
    const uint8 Hue = static_cast<uint8>(GetTypeHash(Message.SemanticType) % 255);
    return FLinearColor::MakeFromHSV8(Hue, 180, 255);
}

float AGeoAreaLayerActor::ResolveOpacity(const FGeoMessageEnvelope& Message) const
{
    const FString Text = Message.Properties.FindRef(TEXT("visual.opacity"));
    if (Text.IsEmpty())
    {
        return 0.22f;
    }
    return FMath::Clamp(FCString::Atof(*Text), 0.04f, 0.85f);
}

bool AGeoAreaLayerActor::IsExpired(const FRenderedGeoArea& Area, double NowSeconds) const
{
    if (Area.ExpireAtSeconds > 0.0)
    {
        return NowSeconds > Area.ExpireAtSeconds;
    }
    return NowSeconds - Area.LastSeenSeconds > AreaLifetimeSeconds;
}

void AGeoAreaLayerActor::RemoveAreaAt(int32 Index)
{
    if (!ActiveAreas.IsValidIndex(Index))
    {
        return;
    }
    const int32 Last = ActiveAreas.Num() - 1;
    EntityToArea.Remove(ActiveAreas[Index].EntityKey);
    if (Index != Last)
    {
        ActiveAreas[Index] = ActiveAreas[Last];
        EntityToArea.Add(ActiveAreas[Index].EntityKey, Index);
    }
    ActiveAreas.RemoveAt(Last, 1, EAllowShrinking::No);
    ++RuntimeStats.IncrementalRemovals;
    bNeedsRebuild = true;
}

int32 AGeoAreaLayerActor::EvictExpiredAreas(double NowSeconds, int32 MaxRemovals)
{
    int32 Removed = 0;
    int32 Index = 0;
    while (Index < ActiveAreas.Num() && Removed < MaxRemovals)
    {
        if (IsExpired(ActiveAreas[Index], NowSeconds))
        {
            RemoveAreaAt(Index);
            ++RuntimeStats.ExpiredRemovals;
            ++Removed;
            continue;
        }
        ++Index;
    }
    return Removed;
}

int32 AGeoAreaLayerActor::TrimToCapacity(int32 MaxRemovals)
{
    int32 Removed = 0;
    while (ActiveAreas.Num() >= MaxVisibleAreas && Removed < MaxRemovals)
    {
        int32 Oldest = 0;
        for (int32 Index = 1; Index < ActiveAreas.Num(); ++Index)
        {
            if (ActiveAreas[Index].LastSeenSeconds < ActiveAreas[Oldest].LastSeenSeconds)
            {
                Oldest = Index;
            }
        }
        RemoveAreaAt(Oldest);
        ++RuntimeStats.CapacityEvictions;
        ++Removed;
    }
    return Removed;
}

void AGeoAreaLayerActor::ProjectRing(const TArray<FGeoPosition>& Ring, double Radius, TArray<FGeoPosition>& OutUnwrapped, TArray<FVector>& OutWorld) const
{
    OutUnwrapped.Reset();
    OutWorld.Reset();
    TArray<FGeoPosition> Work = Ring;
    CloseAndDedup(Work);
    if (Work.Num() < 3)
    {
        return;
    }
    if (Work.Num() > MaxRingVertices)
    {
        TArray<FGeoPosition> Reduced;
        const double Step = static_cast<double>(Work.Num()) / static_cast<double>(MaxRingVertices);
        Reduced.Reserve(MaxRingVertices);
        for (int32 Index = 0; Index < MaxRingVertices; ++Index)
        {
            Reduced.Add(Work[FMath::Clamp(static_cast<int32>(Index * Step), 0, Work.Num() - 1)]);
        }
        Work = MoveTemp(Reduced);
    }

    TArray<FGeoPosition> Dense;
    Dense.Reserve(Work.Num() * 2);
    for (int32 Index = 0; Index < Work.Num(); ++Index)
    {
        const FGeoPosition& From = Work[Index];
        const FGeoPosition& To = Work[(Index + 1) % Work.Num()];
        Dense.Add(From);
        const double Distance = UGeoMathLibrary::GreatCircleDistanceKm(From, To);
        const int32 Splits = FMath::Clamp(static_cast<int32>(Distance / (MaxEdgeDegrees * 111.0)), 0, 6);
        for (int32 Split = 1; Split <= Splits; ++Split)
        {
            Dense.Add(UGeoMathLibrary::GreatCircleInterpolation(From, To, static_cast<double>(Split) / (Splits + 1)));
        }
    }

    OutUnwrapped.Reserve(Dense.Num());
    OutWorld.Reserve(Dense.Num());
    double PreviousLon = Dense[0].Longitude;
    for (const FGeoPosition& Position : Dense)
    {
        FGeoPosition Unwrapped = Position;
        Unwrapped.Longitude = UnwrapDelta(PreviousLon, Position.Longitude);
        PreviousLon = Unwrapped.Longitude;
        OutUnwrapped.Add(Unwrapped);
        OutWorld.Add(GetActorTransform().TransformPosition(
            UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Position.Latitude, Position.Longitude) * Radius));
    }
}

void AGeoAreaLayerActor::AppendOutlineTransforms(TArray<FTransform>& Out, const TArray<FVector>& WorldRing) const
{
    if (WorldRing.Num() < 2)
    {
        return;
    }
    for (int32 Index = 0; Index < WorldRing.Num(); ++Index)
    {
        const FVector& Previous = WorldRing[Index];
        const FVector& Current = WorldRing[(Index + 1) % WorldRing.Num()];
        const FVector Delta = Current - Previous;
        const double Length = Delta.Size();
        if (Length < 0.05)
        {
            continue;
        }
        const FVector Midpoint = (Previous + Current) * 0.5;
        const FRotator Rotation = FRotationMatrix::MakeFromZ(Delta.GetSafeNormal()).Rotator();
        Out.Emplace(Rotation, Midpoint, FVector(OutlineThickness, OutlineThickness, Length / 100.0));
    }
}

void AGeoAreaLayerActor::TessellateRing(const TArray<FGeoPosition>& Unwrapped, const TArray<FVector>& WorldRing, TArray<FVector>& OutVertices, TArray<int32>& OutTriangles) const
{
    if (Unwrapped.Num() < 3 || Unwrapped.Num() != WorldRing.Num())
    {
        return;
    }
    if (OutTriangles.Num() / 3 >= MaxFillTriangles)
    {
        return;
    }
    TArray<int32> Indices;
    Indices.Reserve(Unwrapped.Num());
    for (int32 Index = 0; Index < Unwrapped.Num(); ++Index)
    {
        Indices.Add(Index);
    }
    if (SignedArea(Unwrapped) > 0.0)
    {
        Algo::Reverse(Indices);
    }

    const int32 VertexBase = OutVertices.Num();
    OutVertices.Append(WorldRing);

    auto IsConvex = [&](int32 Prev, int32 Curr, int32 Next)
    {
        const FGeoPosition& A = Unwrapped[Prev];
        const FGeoPosition& B = Unwrapped[Curr];
        const FGeoPosition& C = Unwrapped[Next];
        return (B.Longitude - A.Longitude) * (C.Latitude - A.Latitude) - (B.Latitude - A.Latitude) * (C.Longitude - A.Longitude) > 0.0;
    };

    int32 Guard = Indices.Num() * Indices.Num();
    while (Indices.Num() > 3 && Guard-- > 0)
    {
        bool bClip = false;
        for (int32 I = 0; I < Indices.Num(); ++I)
        {
            const int32 Prev = Indices[(I + Indices.Num() - 1) % Indices.Num()];
            const int32 Curr = Indices[I];
            const int32 Next = Indices[(I + 1) % Indices.Num()];
            if (!IsConvex(Prev, Curr, Next))
            {
                continue;
            }
            bool bEmpty = true;
            for (int32 Other : Indices)
            {
                if (Other == Prev || Other == Curr || Other == Next)
                {
                    continue;
                }
                if (PointInTriangle(Unwrapped[Other], Unwrapped[Prev], Unwrapped[Curr], Unwrapped[Next]))
                {
                    bEmpty = false;
                    break;
                }
            }
            if (!bEmpty)
            {
                continue;
            }
            OutTriangles.Add(VertexBase + Prev);
            OutTriangles.Add(VertexBase + Curr);
            OutTriangles.Add(VertexBase + Next);
            Indices.RemoveAt(I);
            bClip = true;
            break;
        }
        if (!bClip)
        {
            break;
        }
        if (OutTriangles.Num() / 3 >= MaxFillTriangles)
        {
            return;
        }
    }
    if (Indices.Num() >= 3)
    {
        OutTriangles.Add(VertexBase + Indices[0]);
        OutTriangles.Add(VertexBase + Indices[1]);
        OutTriangles.Add(VertexBase + Indices[2]);
    }
}

void AGeoAreaLayerActor::RebuildMeshes()
{
    bNeedsRebuild = false;
    TArray<FVector> Vertices;
    TArray<int32> Triangles;
    TArray<FTransform> Outlines;
    Vertices.Reserve(1024);
    Triangles.Reserve(3072);
    Outlines.Reserve(1024);
    FLinearColor Average = FLinearColor::Transparent;
    float Opacity = 0.22f;
    int32 ColorCount = 0;
    for (const FRenderedGeoArea& Area : ActiveAreas)
    {
        const int32 Rings = Area.Message.Geometry.NumRings();
        for (int32 RingIndex = 0; RingIndex < Rings; ++RingIndex)
        {
            TArray<FGeoPosition> Ring;
            if (!Area.Message.Geometry.GetRing(RingIndex, Ring))
            {
                continue;
            }
            TArray<FGeoPosition> Unwrapped;
            TArray<FVector> WorldFill;
            TArray<FVector> WorldOutline;
            ProjectRing(Ring, GlobeRadius + FillHeight, Unwrapped, WorldFill);
            ProjectRing(Ring, GlobeRadius + OutlineHeight, Unwrapped, WorldOutline);
            if (RingIndex == 0)
            {
                TessellateRing(Unwrapped, WorldFill, Vertices, Triangles);
            }
            AppendOutlineTransforms(Outlines, WorldOutline);
        }
        Average += Area.Color;
        Opacity = Area.Opacity;
        ++ColorCount;
    }

    if (FillMesh)
    {
        FillMesh->ClearAllMeshSections();
        if (Vertices.Num() >= 3 && Triangles.Num() >= 3)
        {
            TArray<FVector> Normals;
            TArray<FVector2D> UVs;
            TArray<FColor> Colors;
            TArray<FProcMeshTangent> Tangents;
            Normals.Reserve(Vertices.Num());
            UVs.Reserve(Vertices.Num());
            for (const FVector& Vertex : Vertices)
            {
                Normals.Add(Vertex.GetSafeNormal());
                UVs.Add(FVector2D::ZeroVector);
            }
            FillMesh->CreateMeshSection(0, Vertices, Triangles, Normals, UVs, Colors, Tangents, false);
            if (FillMaterial)
            {
                if (ColorCount > 0)
                {
                    FillMaterial->SetVectorParameterValue(TEXT("Color"), Average / ColorCount);
                }
                FillMaterial->SetScalarParameterValue(TEXT("Opacity"), Opacity);
                FillMesh->SetMaterial(0, FillMaterial);
            }
        }
    }

    if (OutlineMesh)
    {
        OutlineMesh->ClearInstances();
        if (Outlines.Num() > 0)
        {
            OutlineMesh->AddInstances(Outlines, true, true);
        }
    }
}

void AGeoAreaLayerActor::RefreshSelectionHighlight()
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
    const int32 Rings = Selected.Geometry.NumRings();
    for (int32 RingIndex = 0; RingIndex < Rings; ++RingIndex)
    {
        TArray<FGeoPosition> Ring;
        if (!Selected.Geometry.GetRing(RingIndex, Ring))
        {
            continue;
        }
        TArray<FGeoPosition> Unwrapped;
        TArray<FVector> World;
        ProjectRing(Ring, GlobeRadius + OutlineHeight + 1.0, Unwrapped, World);
        AppendOutlineTransforms(Transforms, World);
    }
    if (Transforms.Num() > 0)
    {
        SelectionMesh->AddInstances(Transforms, true, true);
    }
}

bool AGeoAreaLayerActor::FindClosestMessageToRay(const FVector& RayOrigin, const FVector& RayDirection, double RayLength, double MaxDistance, FGeoMessageEnvelope& OutMessage) const
{
    const FVector RayEnd = RayOrigin + RayDirection.GetSafeNormal() * RayLength;
    double BestDistanceSquared = FMath::Square(MaxDistance);
    bool bFound = false;
    for (const FRenderedGeoArea& Area : ActiveAreas)
    {
        const int32 Rings = Area.Message.Geometry.NumRings();
        for (int32 RingIndex = 0; RingIndex < Rings; ++RingIndex)
        {
            TArray<FGeoPosition> Ring;
            if (!Area.Message.Geometry.GetRing(RingIndex, Ring))
            {
                continue;
            }
            TArray<FGeoPosition> Unwrapped;
            TArray<FVector> World;
            ProjectRing(Ring, GlobeRadius + OutlineHeight, Unwrapped, World);
            for (int32 Index = 0; Index < World.Num(); ++Index)
            {
                FVector ClosestRay;
                FVector ClosestEdge;
                FMath::SegmentDistToSegmentSafe(RayOrigin, RayEnd, World[Index], World[(Index + 1) % World.Num()], ClosestRay, ClosestEdge);
                const double DistanceSquared = FVector::DistSquared(ClosestRay, ClosestEdge);
                if (DistanceSquared < BestDistanceSquared)
                {
                    BestDistanceSquared = DistanceSquared;
                    OutMessage = Area.Message;
                    bFound = true;
                }
            }
        }
    }
    return bFound;
}

void AGeoAreaLayerActor::OnLayerVisibilityChanged(const FString& LayerId, bool bVisible)
{
    if (LayerId == TEXT("core.areas"))
    {
        SetActorHiddenInGame(!bVisible);
    }
}

FGeoLayerManifest AGeoAreaLayerActor::CreateLayerManifest() const
{
    FGeoLayerManifest Manifest;
    Manifest.LayerId = TEXT("core.areas");
    Manifest.DisplayName = TEXT("Geospatial Areas");
    Manifest.GeometryTypes = {EGeoGeometryType::Polygon, EGeoGeometryType::MultiPolygon};
    Manifest.bSupportsAggregation = false;
    return Manifest;
}

FGeoRenderLayerStatistics AGeoAreaLayerActor::GetRenderStatistics() const
{
    FGeoRenderLayerStatistics Stats = RuntimeStats;
    Stats.TrackedItems = ActiveAreas.Num();
    Stats.RenderedInstances = OutlineMesh ? OutlineMesh->GetInstanceCount() : 0;
    return Stats;
}
