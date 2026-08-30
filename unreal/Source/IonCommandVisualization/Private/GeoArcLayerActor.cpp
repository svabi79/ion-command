#include "GeoArcLayerActor.h"

#include "Components/InstancedStaticMeshComponent.h"
#include "Components/SceneComponent.h"
#include "GeoDataSubsystem.h"
#include "GeoLayerSubsystem.h"
#include "GeoMathLibrary.h"
#include "GeoRenderSlotMath.h"
#include "GeoSelectionSubsystem.h"
#include "Materials/MaterialInstanceDynamic.h"
#include "UObject/ConstructorHelpers.h"

namespace
{
constexpr int32 PaletteSize = 11;
}

AGeoArcLayerActor::AGeoArcLayerActor()
{
    PrimaryActorTick.bCanEverTick = true;
    SceneRoot = CreateDefaultSubobject<USceneComponent>(TEXT("Root"));
    SetRootComponent(SceneRoot);
    // Beam segments are engine cubes (12 triangles): at 2-unit beam width the
    // cross-section shape vanishes under bloom, while the smooth BasicShapes
    // cylinder costs three orders of magnitude more triangles - 10k arcs x 16
    // segments made the packaged client primitive-bound at ~10 fps.
    static ConstructorHelpers::FObjectFinder<UStaticMesh> SegmentMesh(TEXT("/Engine/BasicShapes/Cube.Cube"));
    for (int32 Index = 0; Index < PaletteSize; ++Index)
    {
        UInstancedStaticMeshComponent* Instances = CreateDefaultSubobject<UInstancedStaticMeshComponent>(*FString::Printf(TEXT("ArcInstances_%02d"), Index));
        Instances->SetupAttachment(SceneRoot);
        Instances->SetStaticMesh(SegmentMesh.Object);
        Instances->SetCollisionEnabled(ECollisionEnabled::NoCollision);
        Instances->SetCastShadow(false);
        // Custom data 0/1 carry spawn time and 1/lifetime for the GPU fade;
        // 2 carries the endpoint-congestion brightness (0 reads as 1 in the
        // material so meshes without the slot stay at full brightness).
        Instances->SetNumCustomDataFloats(3);
        if (UMaterialInterface* SignalMaterial = LoadObject<UMaterialInterface>(nullptr, *FString::Printf(TEXT("/Game/ION/Materials/MI_Signal_%02d.MI_Signal_%02d"), Index, Index)))
        {
            Instances->SetMaterial(0, SignalMaterial);
        }
        PaletteMeshes.Add(Instances);
    }
    // One CPU bucket per palette component, index-parallel with its instance
    // blocks for the lifetime of the actor (PaletteMeshes.Num() never
    // changes after construction).
    ArcsByPalette.SetNum(PaletteMeshes.Num());
    SelectionMesh = CreateDefaultSubobject<UInstancedStaticMeshComponent>(TEXT("SelectedArcInstances"));
    SelectionMesh->SetupAttachment(SceneRoot);
    SelectionMesh->SetStaticMesh(SegmentMesh.Object);
    SelectionMesh->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    SelectionMesh->SetCastShadow(false);
    // Custom data 0 = position along the path for the travelling pulse.
    SelectionMesh->SetNumCustomDataFloats(1);
    if (UMaterialInterface* SelectionMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/M_SelectedPath.M_SelectedPath")))
    {
        SelectionMesh->SetMaterial(0, SelectionMaterial);
    }
    static ConstructorHelpers::FObjectFinder<UStaticMesh> SphereMesh(TEXT("/Engine/BasicShapes/Sphere.Sphere"));
    EndpointMesh = CreateDefaultSubobject<UInstancedStaticMeshComponent>(TEXT("SelectedEndpointInstances"));
    EndpointMesh->SetupAttachment(SceneRoot);
    EndpointMesh->SetStaticMesh(SphereMesh.Object);
    EndpointMesh->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    EndpointMesh->SetCastShadow(false);
    if (UMaterialInterface* SelectionMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Signal_Selected.MI_Signal_Selected")))
    {
        EndpointMesh->SetMaterial(0, SelectionMaterial);
    }
}

void AGeoArcLayerActor::OnConstruction(const FTransform& Transform)
{
    Super::OnConstruction(Transform);
    if (GetWorld() && !GetWorld()->IsGameWorld())
    {
        BuildEditorPreview();
    }
}

void AGeoArcLayerActor::PostRegisterAllComponents()
{
    Super::PostRegisterAllComponents();
    if (GetWorld() && !GetWorld()->IsGameWorld()) BuildEditorPreview();
}

void AGeoArcLayerActor::BuildEditorPreview()
{
    for (UInstancedStaticMeshComponent* Instances : PaletteMeshes) Instances->ClearInstances();
    SelectionMesh->ClearInstances();

    constexpr int32 PreviewArcCount = 96;
    for (int32 Index = 0; Index < PreviewArcCount; ++Index)
    {
        FGeoMessageEnvelope Preview;
        Preview.Geometry.Type = EGeoGeometryType::GreatCircle;

        FGeoPosition From;
        From.Longitude = FMath::Fmod(Index * 137.507764, 360.0) - 180.0;
        From.Latitude = FMath::Sin(Index * 0.73) * 58.0;
        Preview.Geometry.Positions.Add(From);

        FGeoPosition To;
        To.Longitude = FMath::Fmod(From.Longitude + 248.0 + (Index % 9) * 17.0 + 540.0, 360.0) - 180.0;
        To.Latitude = FMath::Sin(Index * 0.47 + 1.7) * 52.0;
        Preview.Geometry.Positions.Add(To);

        const int32 PaletteIndex = Index % PaletteMeshes.Num();
        const double Thickness = ArcThickness * (0.8 + (Index % 5) * 0.2);
        AddArcInstancesTo(PaletteMeshes[PaletteIndex], Preview, Thickness);
    }
}

void AGeoArcLayerActor::BeginPlay()
{
    Super::BeginPlay();
    Reset();
    PaletteMaterials.Reset();
    PaletteBaseIntensities.Reset();
    for (int32 Index = 0; Index < PaletteMeshes.Num(); ++Index)
    {
        UMaterialInstanceDynamic* Material = PaletteMeshes[Index]->CreateAndSetMaterialInstanceDynamic(0);
        float BaseIntensity = 3.4f;
        if (Material)
        {
            Material->SetVectorParameterValue(TEXT("Color"), ResolvePaletteColor(Index));
            Material->GetScalarParameterValue(FMaterialParameterInfo(TEXT("Intensity")), BaseIntensity);
        }
        PaletteMaterials.Add(Material);
        PaletteBaseIntensities.Add(BaseIntensity);
    }
    if (UMaterialInstanceDynamic* Material = SelectionMesh->CreateAndSetMaterialInstanceDynamic(0))
    {
        Material->SetVectorParameterValue(TEXT("Color"), FLinearColor(0.82f, 1.0f, 1.0f));
    }
    if (UMaterialInstanceDynamic* Material = EndpointMesh->CreateAndSetMaterialInstanceDynamic(0))
    {
        Material->SetVectorParameterValue(TEXT("Color"), FLinearColor(1.0f, 1.0f, 1.0f));
    }
    if (UGameInstance* GameInstance = GetGameInstance())
    {
        DataSubsystem = GameInstance->GetSubsystem<UGeoDataSubsystem>();
        if (DataSubsystem.IsValid())
        {
            DataSubsystem->OnMessageAccepted().AddUObject(this, &AGeoArcLayerActor::OnMessageAccepted);
            DataSubsystem->OnDataReset().AddUObject(this, &AGeoArcLayerActor::Reset);
            for (const FGeoMessageEnvelope& Message : DataSubsystem->GetActiveMessages()) OnMessageAccepted(Message);
        }
        if (UGeoLayerSubsystem* LayerSubsystem = GameInstance->GetSubsystem<UGeoLayerSubsystem>())
        {
            const FGeoLayerManifest Manifest = CreateLayerManifest();
            LayerSubsystem->RegisterLayer(Manifest);
            LayerSubsystem->OnLayerVisibilityChanged().AddUObject(this, &AGeoArcLayerActor::OnLayerVisibilityChanged);
            SetActorHiddenInGame(!LayerSubsystem->IsLayerVisible(Manifest.LayerId));
        }
    }
}

void AGeoArcLayerActor::EndPlay(const EEndPlayReason::Type EndPlayReason)
{
    if (DataSubsystem.IsValid()) { DataSubsystem->OnMessageAccepted().RemoveAll(this); DataSubsystem->OnDataReset().RemoveAll(this); }
    if (UGameInstance* GameInstance = GetGameInstance()) if (UGeoLayerSubsystem* LayerSubsystem = GameInstance->GetSubsystem<UGeoLayerSubsystem>()) LayerSubsystem->OnLayerVisibilityChanged().RemoveAll(this);
    Super::EndPlay(EndPlayReason);
}

void AGeoArcLayerActor::UpdateZoomResponse()
{
    const APlayerController* Player = GetWorld() ? GetWorld()->GetFirstPlayerController() : nullptr;
    if (!Player || !Player->PlayerCameraManager)
    {
        return;
    }
    // Distance from the camera to the surface it is looking at, which is what
    // decides how large a fixed world size appears and how much of the arc
    // weave is packed into the frame.
    const double Altitude = FMath::Max(0.0, Player->PlayerCameraManager->GetCameraLocation().Length() - 1000.0);
    // Normalised against the default orbit, so the view the operator already
    // likes is exactly 1.0 and nothing about it changes.
    const double Orbit = FMath::Clamp(Altitude / 2400.0, 0.0, 1.0);

    // Thickness tracks the distance almost directly: that is what keeps a
    // segment about the same width on screen instead of fattening as the
    // camera descends. The floor stops it vanishing entirely.
    const float ZoomThickness = static_cast<float>(FMath::Max(0.012, Orbit));
    // Brightness falls faster than linearly. Additive arcs stack, and close
    // in there are still thousands of them crossing the frame, so halving
    // the distance has to do more than halve the light.
    const float ZoomDim = static_cast<float>(FMath::Max(0.05, FMath::Pow(Orbit, 1.6)));

    if (FMath::IsNearlyEqual(ZoomDim, LastZoomDim, 0.002f))
    {
        return;
    }
    LastZoomDim = ZoomDim;
    for (int32 Index = 0; Index < PaletteMeshes.Num(); ++Index)
    {
        if (!PaletteMaterials.IsValidIndex(Index) || !PaletteMaterials[Index])
        {
            continue;
        }
        PaletteMaterials[Index]->SetScalarParameterValue(TEXT("ZoomDim"), ZoomDim);
        PaletteMaterials[Index]->SetScalarParameterValue(TEXT("ZoomThickness"), ZoomThickness);
    }
}

void AGeoArcLayerActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    UpdateZoomResponse();
    RefreshSelectionHighlight();
    const double Now = FPlatformTime::Seconds();
    if (Now - LastExpiryCheck < 3.0) return;
    LastExpiryCheck = Now;
    // Visual lifetime runs on the render clock (arrival time), exactly like
    // the GPU fade. Observed timestamps can be minutes late on live feeds and
    // would remove arcs mid-fade. The half-second grace guarantees an arc is
    // fully invisible before its instances are dropped.
    const double RenderNow = GetWorld() ? GetWorld()->GetTimeSeconds() : 0.0;
    // Endpoint congestion decays on the same cadence so a hotspot recovers
    // full brightness once its traffic subsides.
    for (auto It = EndpointDensity.CreateIterator(); It; ++It)
    {
        It->Value *= 0.85f;
        if (It->Value < 0.5f) It.RemoveCurrent();
    }
    // Each removal only touches the arc actually leaving (see
    // GeoRenderSlotMath.h), so unlike the old threshold-gated batch there is
    // no performance reason to wait for a minimum number of expired arcs to
    // accumulate. The bound below is only a safety valve against a
    // pathological simultaneous-expiry burst (e.g. the client sat
    // backgrounded for a while); anything left over is already fully faded
    // and invisible, and is picked up by the next check.
    const int32 MaxRemovalsThisCheck = FMath::Max(256, MaxVisibleArcs / 4);
    EvictExpiredArcs(RenderNow, MaxRemovalsThisCheck);
}

bool AGeoArcLayerActor::Supports(const FGeoMessageEnvelope& Message) const
{
    return (Message.Geometry.Type == EGeoGeometryType::GreatCircle || Message.Geometry.Type == EGeoGeometryType::Arc) && Message.Geometry.Positions.Num() >= 2;
}

void AGeoArcLayerActor::Submit(const FGeoMessageEnvelope& Message)
{
    if (!Supports(Message)) return;
    if (EntityFilter.Num() > 0 && !EntityFilter.Contains(Message.FromEntityId) && !EntityFilter.Contains(Message.ToEntityId))
    {
        return;
    }
    if (!PropertyFilterValue.IsEmpty() && Message.Properties.FindRef(PropertyFilterKey) != PropertyFilterValue)
    {
        return;
    }
    if (TotalArcCount >= MaxVisibleArcs)
    {
        // Evict the oldest chunk incrementally (swap-and-pop per arc)
        // instead of rebuilding: at firehose rates every submit would
        // otherwise touch every surviving arc's instances just to make room
        // for one more.
        TrimToCapacity(FMath::Max(1, MaxVisibleArcs / 5));
    }
    FRenderedGeoArc Arc;
    Arc.Message = Message;
    Arc.AddedUtc = Message.Time.ObservedUtc.GetTicks() > 0 ? Message.Time.ObservedUtc : FDateTime::UtcNow();
    Arc.SpawnTimeSeconds = GetWorld() ? GetWorld()->GetTimeSeconds() : 0.0;
    // Resolved once, clamped to a real component, and stored on the record
    // itself so the palette a subclass's ResolvePaletteIndex() chose always
    // matches the ArcsByPalette bucket the arc actually lives in.
    const int32 ResolvedPalette = ResolvePaletteIndex(Message);
    const int32 PaletteIndex = PaletteMeshes.IsValidIndex(ResolvedPalette) ? ResolvedPalette : PaletteMeshes.Num() - 1;
    Arc.PaletteIndex = PaletteIndex;
    // Endpoint congestion (a skimmer hearing hundreds of stations) dims the
    // segments near that endpoint with the square root of its arc count;
    // the rest of the path keeps full brightness and color.
    auto BumpDensity = [this](const FString& EntityId) -> float
    {
        if (EntityId.IsEmpty()) return 0.0f;
        if (float* Density = EndpointDensity.Find(EntityId)) return *Density += 1.0f;
        if (EndpointDensity.Num() >= 8192) return 0.0f;
        return EndpointDensity.Add(EntityId, 1.0f);
    };
    auto EndBrightness = [](float Density)
    {
        return Density > 16.0f ? FMath::Clamp(4.0f / FMath::Sqrt(Density), 0.25f, 1.0f) : 1.0f;
    };
    Arc.BrightnessFrom = EndBrightness(BumpDensity(Message.FromEntityId));
    Arc.BrightnessTo = EndBrightness(BumpDensity(Message.ToEntityId));
    AddArc(PaletteIndex, Arc);
}

void AGeoArcLayerActor::Reset()
{
    for (TArray<FRenderedGeoArc>& Arcs : ArcsByPalette) Arcs.Reset();
    TotalArcCount = 0;
    EndpointDensity.Reset();
    for (UInstancedStaticMeshComponent* Instances : PaletteMeshes) Instances->ClearInstances();
    SelectionMesh->ClearInstances();
    EndpointMesh->ClearInstances();
    HighlightedMessageId.Reset();
    ApplySelectionDimming(false);
}

void AGeoArcLayerActor::OnMessageAccepted(const FGeoMessageEnvelope& Message) { Submit(Message); }

void AGeoArcLayerActor::SetBandFocus(int32 PaletteIndex)
{
    BandFocusIndex = (PaletteIndex == BandFocusIndex) ? INDEX_NONE : PaletteIndex;
    for (int32 Index = 0; Index < PaletteMeshes.Num(); ++Index)
    {
        PaletteMeshes[Index]->SetVisibility(BandFocusIndex == INDEX_NONE || Index == BandFocusIndex);
    }
}

void AGeoArcLayerActor::SetEntityFilter(const TArray<FString>& EntityIds)
{
    if (EntityFilter == EntityIds) return;
    EntityFilter = EntityIds;
    Reset();
    if (DataSubsystem.IsValid())
    {
        // A deliberate, infrequent operator action, not periodic churn: this
        // resubmits the whole active window through the same incremental
        // Submit() path used for live traffic, so it never falls back to a
        // bulk clear-and-readd of an existing population - but it still
        // touches every message in the window once, hence counted here.
        ++RuntimeStats.FullRebuilds;
        for (const FGeoMessageEnvelope& Message : DataSubsystem->GetActiveMessages()) Submit(Message);
    }
}

void AGeoArcLayerActor::SetPropertyFilter(const FString& Key, const FString& Value)
{
    if (PropertyFilterKey == Key && PropertyFilterValue == Value) return;
    PropertyFilterKey = Key;
    PropertyFilterValue = Value;
    Reset();
    if (DataSubsystem.IsValid())
    {
        ++RuntimeStats.FullRebuilds;
        for (const FGeoMessageEnvelope& Message : DataSubsystem->GetActiveMessages()) Submit(Message);
    }
}

int32 AGeoArcLayerActor::AddArc(int32 PaletteIndex, const FRenderedGeoArc& Arc)
{
    UInstancedStaticMeshComponent* Instances = PaletteMeshes[PaletteIndex];
    TArray<FTransform> Transforms;
    Transforms.Reserve(SegmentsPerArc);
    AppendArcTransforms(Transforms, Arc.Message, ArcThickness);
    const TArray<int32> Indices = Instances->AddInstances(Transforms, true, true);
    const float InverseLifetime = LifetimeSeconds > 0.0 ? static_cast<float>(1.0 / LifetimeSeconds) : 0.0f;
    for (int32 Segment = 0; Segment < Indices.Num(); ++Segment)
    {
        Instances->SetCustomDataValue(Indices[Segment], 0, static_cast<float>(Arc.SpawnTimeSeconds), false);
        Instances->SetCustomDataValue(Indices[Segment], 1, InverseLifetime, false);
        Instances->SetCustomDataValue(Indices[Segment], 2, SegmentBrightness(Arc, Segment), false);
    }
    Instances->MarkRenderStateDirty();
    // Append to the CPU bucket last: AddInstances() above already grew the
    // component by SegmentsPerArc, so appending here is what keeps
    // ArcsByPalette[PaletteIndex].Num() * SegmentsPerArc ==
    // Instances->GetInstanceCount() true again.
    const int32 Slot = ArcsByPalette[PaletteIndex].Add(Arc);
    ++TotalArcCount;
    ++RuntimeStats.IncrementalInserts;
    return Slot;
}

void AGeoArcLayerActor::RemoveArcAtSlot(int32 PaletteIndex, int32 Slot)
{
    TArray<FRenderedGeoArc>& Arcs = ArcsByPalette[PaletteIndex];
    UInstancedStaticMeshComponent* Instances = PaletteMeshes[PaletteIndex];
    const int32 BlockCountBeforeFree = Arcs.Num();
    int32 MovedFromSlot = INDEX_NONE;
    if (GeoRenderSlotMath::ComputeFreeMove(Slot, BlockCountBeforeFree, MovedFromSlot))
    {
        // Recompute the relocated arc's transforms/custom data from its own
        // source record rather than reading the instance buffer back: it is
        // cheaper, and it cannot inherit any staleness the buffer might
        // have accumulated.
        const FRenderedGeoArc& MovedArc = Arcs[MovedFromSlot];
        TArray<FTransform> Transforms;
        Transforms.Reserve(SegmentsPerArc);
        AppendArcTransforms(Transforms, MovedArc.Message, ArcThickness);
        const int32 BaseInstance = Slot * SegmentsPerArc;
        Instances->BatchUpdateInstancesTransforms(BaseInstance, Transforms, /*bWorldSpace=*/true, /*bMarkRenderStateDirty=*/false, /*bTeleport=*/true);
        const float InverseLifetime = LifetimeSeconds > 0.0 ? static_cast<float>(1.0 / LifetimeSeconds) : 0.0f;
        for (int32 Segment = 0; Segment < SegmentsPerArc; ++Segment)
        {
            Instances->SetCustomDataValue(BaseInstance + Segment, 0, static_cast<float>(MovedArc.SpawnTimeSeconds), false);
            Instances->SetCustomDataValue(BaseInstance + Segment, 1, InverseLifetime, false);
            Instances->SetCustomDataValue(BaseInstance + Segment, 2, SegmentBrightness(MovedArc, Segment), false);
        }
        // Keep the CPU record's own slot in sync with the instances we just
        // wrote. MovedArc is read-only above; reassigning here (rather than
        // moving) keeps the code simple - Arcs.RemoveAt() below discards the
        // source element regardless.
        Arcs[Slot] = Arcs[MovedFromSlot];
    }
    Arcs.RemoveAt(BlockCountBeforeFree - 1, 1, EAllowShrinking::No);
    // Trim the trailing block: now either a stale duplicate (move case) or
    // the freed block itself (no-move case). Removed from the component's
    // true tail in descending order, which is safe regardless of whether
    // RemoveInstance() internally swaps-with-last (it does, per
    // r.InstancedStaticMeshes.ForceRemoveAtSwap) because nothing below the
    // tail block ever changes index when only the tail is removed.
    const int32 TailBase = (BlockCountBeforeFree - 1) * SegmentsPerArc;
    TArray<int32> ToRemove;
    ToRemove.Reserve(SegmentsPerArc);
    for (int32 Segment = SegmentsPerArc - 1; Segment >= 0; --Segment) ToRemove.Add(TailBase + Segment);
    Instances->RemoveInstances(ToRemove, /*bInstanceArrayAlreadySortedInReverseOrder=*/true);
    Instances->MarkRenderStateDirty();
    --TotalArcCount;
    ++RuntimeStats.IncrementalRemovals;
}

int32 AGeoArcLayerActor::EvictExpiredArcs(double RenderNow, int32 MaxRemovals)
{
    const double AgeLimit = LifetimeSeconds + 0.5;
    int32 Removed = 0;
    for (int32 PaletteIndex = 0; PaletteIndex < ArcsByPalette.Num() && Removed < MaxRemovals; ++PaletteIndex)
    {
        TArray<FRenderedGeoArc>& Arcs = ArcsByPalette[PaletteIndex];
        int32 Slot = 0;
        while (Slot < Arcs.Num() && Removed < MaxRemovals)
        {
            if (RenderNow - Arcs[Slot].SpawnTimeSeconds > AgeLimit)
            {
                RemoveArcAtSlot(PaletteIndex, Slot);
                ++Removed;
                ++RuntimeStats.ExpiredRemovals;
                // Do not advance Slot: the arc swapped into this position
                // (if the removed one was not already the run's last block)
                // has not been tested yet.
            }
            else
            {
                ++Slot;
            }
        }
    }
    return Removed;
}

int32 AGeoArcLayerActor::TrimToCapacity(int32 MaxRemovals)
{
    struct FVictim
    {
        int32 PaletteIndex;
        int32 Slot;
        double SpawnTimeSeconds;
    };
    TArray<FVictim> Candidates;
    Candidates.Reserve(TotalArcCount);
    for (int32 PaletteIndex = 0; PaletteIndex < ArcsByPalette.Num(); ++PaletteIndex)
    {
        const TArray<FRenderedGeoArc>& Arcs = ArcsByPalette[PaletteIndex];
        for (int32 Slot = 0; Slot < Arcs.Num(); ++Slot)
        {
            Candidates.Add(FVictim{PaletteIndex, Slot, Arcs[Slot].SpawnTimeSeconds});
        }
    }
    const int32 RemoveCount = FMath::Min(MaxRemovals, Candidates.Num());
    if (RemoveCount <= 0) return 0;
    // Oldest first, matching the original trim's intent: sustained overload
    // sheds the arcs already closest to their natural age-based expiry.
    // This full sort is CPU-only (small structs, no instance/GPU touch) and
    // only runs on the rare burst path - the comment on MaxVisibleArcs notes
    // steady-state traffic should not reach this at all.
    Candidates.Sort([](const FVictim& A, const FVictim& B) { return A.SpawnTimeSeconds < B.SpawnTimeSeconds; });
    Candidates.SetNum(RemoveCount, EAllowShrinking::No);
    // Re-sort the chosen victims so each palette's are processed from the
    // highest slot down. A swap-and-pop removal only ever relocates the
    // CURRENT last slot of that palette, and every not-yet-processed
    // victim's slot is strictly below the one being removed, so it can
    // never be the one relocated out from under a later iteration (proved
    // in GeoRenderSlotMathTests.cpp's descending-batch-removal case).
    Candidates.Sort([](const FVictim& A, const FVictim& B)
    {
        if (A.PaletteIndex != B.PaletteIndex) return A.PaletteIndex < B.PaletteIndex;
        return A.Slot > B.Slot;
    });
    for (const FVictim& Victim : Candidates)
    {
        RemoveArcAtSlot(Victim.PaletteIndex, Victim.Slot);
        ++RuntimeStats.CapacityEvictions;
    }
    return RemoveCount;
}

float AGeoArcLayerActor::SegmentBrightness(const FRenderedGeoArc& Arc, int32 Segment) const
{
    // Fade in over the outer 40% toward each congested end; mid-path
    // segments always render at full brightness.
    const float Alpha = (Segment + 0.5f) / FMath::Max(SegmentsPerArc, 1);
    const float ToRamp = FMath::Clamp((Alpha - 0.6f) / 0.4f, 0.0f, 1.0f);
    const float FromRamp = FMath::Clamp((0.4f - Alpha) / 0.4f, 0.0f, 1.0f);
    const float ToFactor = FMath::Lerp(1.0f, Arc.BrightnessTo, ToRamp);
    const float FromFactor = FMath::Lerp(1.0f, Arc.BrightnessFrom, FromRamp);
    return FMath::Min(ToFactor, FromFactor);
}

void AGeoArcLayerActor::AddArcInstancesTo(UInstancedStaticMeshComponent* Instances, const FGeoMessageEnvelope& Message, double Thickness, bool bWritePathAlpha)
{
    TArray<FTransform> Transforms;
    Transforms.Reserve(SegmentsPerArc);
    AppendArcTransforms(Transforms, Message, Thickness);
    const TArray<int32> Indices = Instances->AddInstances(Transforms, true, true);
    if (bWritePathAlpha)
    {
        // Custom data 0 carries the position along the path so the selected
        // path material can run its energy pulse from TX to RX.
        for (int32 Segment = 0; Segment < Indices.Num(); ++Segment)
        {
            Instances->SetCustomDataValue(Indices[Segment], 0, (Segment + 0.5f) / FMath::Max(SegmentsPerArc, 1), false);
        }
        Instances->MarkRenderStateDirty();
    }
}

void AGeoArcLayerActor::AppendArcTransforms(TArray<FTransform>& Out, const FGeoMessageEnvelope& Message, double Thickness) const
{
    FVector Previous = CalculateArcPoint(Message, 0.0);
    for (int32 Segment = 1; Segment <= SegmentsPerArc; ++Segment)
    {
        const double Alpha = static_cast<double>(Segment) / SegmentsPerArc;
        const FVector Current = CalculateArcPoint(Message, Alpha);
        const FVector Delta = Current - Previous;
        const double Length = Delta.Size();
        const FVector Midpoint = (Previous + Current) * 0.5;
        const FRotator Rotation = FRotationMatrix::MakeFromZ(Delta.GetSafeNormal()).Rotator();
        Out.Emplace(Rotation, Midpoint, FVector(Thickness, Thickness, Length / 100.0));
        Previous = Current;
    }
}

FVector AGeoArcLayerActor::CalculateArcPoint(const FGeoMessageEnvelope& Message, double Alpha) const
{
    const FGeoPosition& From = Message.Geometry.Positions[0];
    const FGeoPosition& To = Message.Geometry.Positions.Last();
    const double DistanceKm = UGeoMathLibrary::GreatCircleDistanceKm(From, To);
    const double NormalizedDistance = FMath::Clamp(DistanceKm / 20000.0, 0.0, 1.0);
    // Keep even 20,000 km paths hugging the planet: tall arcs read as orbit
    // rings instead of propagation paths once they clear the atmosphere.
    const double ArcHeight = FMath::Lerp(22.0, 165.0, FMath::Sin(NormalizedDistance * UE_PI));
    const FGeoPosition Position = UGeoMathLibrary::GreatCircleInterpolation(From, To, Alpha);
    const double Radius = GlobeRadius + FMath::Sin(Alpha * UE_PI) * ArcHeight;
    return GetActorTransform().TransformPosition(UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Position.Latitude, Position.Longitude) * Radius);
}

bool AGeoArcLayerActor::FindClosestMessageToRay(const FVector& RayOrigin, const FVector& RayDirection, double RayLength, double MaxDistance, FGeoMessageEnvelope& OutMessage) const
{
    const FVector RayEnd = RayOrigin + RayDirection.GetSafeNormal() * RayLength;
    double BestDistanceSquared = FMath::Square(MaxDistance);
    bool bFound = false;
    for (const TArray<FRenderedGeoArc>& Arcs : ArcsByPalette)
    {
        for (const FRenderedGeoArc& Arc : Arcs)
        {
            FVector Previous = CalculateArcPoint(Arc.Message, 0.0);
            for (int32 Segment = 1; Segment <= SegmentsPerArc; ++Segment)
            {
                const FVector Current = CalculateArcPoint(Arc.Message, static_cast<double>(Segment) / SegmentsPerArc);
                FVector ClosestRay;
                FVector ClosestArc;
                FMath::SegmentDistToSegmentSafe(RayOrigin, RayEnd, Previous, Current, ClosestRay, ClosestArc);
                const double DistanceSquared = FVector::DistSquared(ClosestRay, ClosestArc);
                if (DistanceSquared < BestDistanceSquared)
                {
                    BestDistanceSquared = DistanceSquared;
                    OutMessage = Arc.Message;
                    bFound = true;
                }
                Previous = Current;
            }
        }
    }
    return bFound;
}

void AGeoArcLayerActor::RefreshSelectionHighlight()
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
    EndpointMesh->ClearInstances();
    if (!HighlightedMessageId.IsEmpty())
    {
        AddArcInstancesTo(SelectionMesh, Selected, SelectedArcThickness, true);
        const FVector FromLocation = CalculateArcPoint(Selected, 0.0);
        const FVector ToLocation = CalculateArcPoint(Selected, 1.0);
        EndpointMesh->AddInstance(FTransform(FQuat::Identity, FromLocation, FVector(0.15)), true);
        EndpointMesh->AddInstance(FTransform(FQuat::Identity, ToLocation, FVector(0.15)), true);
    }
    ApplySelectionDimming(!HighlightedMessageId.IsEmpty());
}

void AGeoArcLayerActor::ApplySelectionDimming(bool bDim)
{
    if (bSelectionDimmed == bDim) return;
    bSelectionDimmed = bDim;
    for (int32 Index = 0; Index < PaletteMaterials.Num(); ++Index)
    {
        UMaterialInstanceDynamic* Material = PaletteMaterials[Index];
        if (!Material) continue;
        const float BaseIntensity = PaletteBaseIntensities.IsValidIndex(Index) ? PaletteBaseIntensities[Index] : 3.4f;
        Material->SetScalarParameterValue(TEXT("Intensity"), bDim ? BaseIntensity * 0.22f : BaseIntensity);
    }
}

void AGeoArcLayerActor::OnLayerVisibilityChanged(const FString& LayerId, bool bVisible)
{
    if (LayerId == CreateLayerManifest().LayerId) SetActorHiddenInGame(!bVisible);
}

int32 AGeoArcLayerActor::ResolvePaletteIndex(const FGeoMessageEnvelope& Message) const
{
    const FString ExplicitPalette = Message.Properties.FindRef(TEXT("visual.paletteIndex"));
    if (!ExplicitPalette.IsEmpty()) return FMath::Clamp(FCString::Atoi(*ExplicitPalette), 0, PaletteSize - 1);
    return static_cast<int32>(GetTypeHash(Message.SemanticType) % static_cast<uint32>(PaletteSize));
}

FLinearColor AGeoArcLayerActor::ResolvePaletteColor(int32 PaletteIndex) const
{
    const uint8 Hue = static_cast<uint8>((PaletteIndex * 255) / PaletteSize);
    return FLinearColor::MakeFromHSV8(Hue, 185, 255);
}

FString AGeoArcLayerActor::ResolvePaletteLabel(int32 PaletteIndex) const
{
    return FString();
}

FString AGeoArcLayerActor::ResolveTrafficPanelTitle() const
{
    return TEXT("TRAFFIC BY CLASS");
}

void AGeoArcLayerActor::GetPaletteBreakdown(TArray<FGeoPaletteBreakdownEntry>& OutEntries) const
{
    TArray<int32> Counts;
    Counts.SetNumZeroed(PaletteMeshes.Num());
    for (int32 PaletteIndex = 0; PaletteIndex < ArcsByPalette.Num(); ++PaletteIndex)
    {
        if (Counts.IsValidIndex(PaletteIndex)) Counts[PaletteIndex] = ArcsByPalette[PaletteIndex].Num();
    }
    OutEntries.Reset();
    for (int32 Index = 0; Index < Counts.Num(); ++Index)
    {
        FGeoPaletteBreakdownEntry Entry;
        Entry.PaletteIndex = Index;
        Entry.Label = ResolvePaletteLabel(Index);
        Entry.Color = ResolvePaletteColor(Index);
        Entry.Count = Counts[Index];
        if (!Entry.Label.IsEmpty() || Entry.Count > 0) OutEntries.Add(Entry);
    }
}

FGeoLayerManifest AGeoArcLayerActor::CreateLayerManifest() const
{
    FGeoLayerManifest Manifest;
    Manifest.LayerId = TEXT("core.live-arcs");
    Manifest.DisplayName = TEXT("Live Geospatial Arcs");
    Manifest.GeometryTypes = {EGeoGeometryType::GreatCircle, EGeoGeometryType::Arc};
    Manifest.bSupportsAggregation = true;
    return Manifest;
}

FGeoRenderLayerStatistics AGeoArcLayerActor::GetRenderStatistics() const
{
    FGeoRenderLayerStatistics Stats = RuntimeStats;
    Stats.TrackedItems = TotalArcCount;
    int32 InstanceTotal = 0;
    for (const UInstancedStaticMeshComponent* Instances : PaletteMeshes)
    {
        if (Instances) InstanceTotal += Instances->GetInstanceCount();
    }
    Stats.RenderedInstances = InstanceTotal;
    return Stats;
}
