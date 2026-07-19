#include "GeoArcLayerActor.h"

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
    SelectionMesh = CreateDefaultSubobject<UInstancedStaticMeshComponent>(TEXT("SelectedArcInstances"));
    SelectionMesh->SetupAttachment(SceneRoot);
    SelectionMesh->SetStaticMesh(SegmentMesh.Object);
    SelectionMesh->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    SelectionMesh->SetCastShadow(false);
    if (UMaterialInterface* SelectionMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Signal_Selected.MI_Signal_Selected")))
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

void AGeoArcLayerActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    RefreshSelectionHighlight();
    if (bNeedsRebuild)
    {
        bNeedsRebuild = false;
        RebuildInstances();
    }
    const double Now = FPlatformTime::Seconds();
    if (Now - LastExpiryCheck < 3.0) return;
    LastExpiryCheck = Now;
    // Visual lifetime runs on the render clock (arrival time), exactly like
    // the GPU fade. Observed timestamps can be minutes late on live feeds and
    // would remove arcs mid-fade. The half-second grace guarantees an arc is
    // fully invisible before its instances are dropped.
    const double RenderNow = GetWorld() ? GetWorld()->GetTimeSeconds() : 0.0;
    const double AgeLimit = LifetimeSeconds + 0.5;
    // Endpoint congestion decays on the same cadence so a hotspot recovers
    // full brightness once its traffic subsides.
    for (auto It = EndpointDensity.CreateIterator(); It; ++It)
    {
        It->Value *= 0.85f;
        if (It->Value < 0.5f) It.RemoveCurrent();
    }
    int32 ExpiredCount = 0;
    for (const FRenderedGeoArc& Arc : ActiveArcs) if (RenderNow - Arc.SpawnTimeSeconds > AgeLimit) ++ExpiredCount;
    if (ExpiredCount > FMath::Max(32, ActiveArcs.Num() / 20))
    {
        ActiveArcs.RemoveAll([RenderNow, AgeLimit](const FRenderedGeoArc& Arc) { return RenderNow - Arc.SpawnTimeSeconds > AgeLimit; });
        RebuildInstances();
    }
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
    if (ActiveArcs.Num() >= MaxVisibleArcs)
    {
        // Trim without rebuilding: at firehose rates every submit would
        // otherwise trigger a full instance rebuild. One rebuild per frame.
        ActiveArcs.RemoveAt(0, FMath::Max(1, MaxVisibleArcs / 5), EAllowShrinking::No);
        bNeedsRebuild = true;
    }
    FRenderedGeoArc& Arc = ActiveArcs.AddDefaulted_GetRef();
    Arc.Message = Message;
    Arc.AddedUtc = Message.Time.ObservedUtc.GetTicks() > 0 ? Message.Time.ObservedUtc : FDateTime::UtcNow();
    Arc.SpawnTimeSeconds = GetWorld() ? GetWorld()->GetTimeSeconds() : 0.0;
    Arc.PaletteIndex = ResolvePaletteIndex(Message);
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
    if (!bNeedsRebuild)
    {
        AddArcInstances(Arc);
    }
}

void AGeoArcLayerActor::Reset()
{
    ActiveArcs.Reset();
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
        for (const FGeoMessageEnvelope& Message : DataSubsystem->GetActiveMessages()) Submit(Message);
    }
}

void AGeoArcLayerActor::AddArcInstances(const FRenderedGeoArc& Arc)
{
    UInstancedStaticMeshComponent* Instances = PaletteMeshes.IsValidIndex(Arc.PaletteIndex) ? PaletteMeshes[Arc.PaletteIndex] : PaletteMeshes.Last();
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

void AGeoArcLayerActor::AddArcInstancesTo(UInstancedStaticMeshComponent* Instances, const FGeoMessageEnvelope& Message, double Thickness)
{
    TArray<FTransform> Transforms;
    Transforms.Reserve(SegmentsPerArc);
    AppendArcTransforms(Transforms, Message, Thickness);
    Instances->AddInstances(Transforms, false, true);
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
    for (const FRenderedGeoArc& Arc : ActiveArcs)
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
        AddArcInstancesTo(SelectionMesh, Selected, SelectedArcThickness);
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

void AGeoArcLayerActor::RebuildInstances()
{
    // One bulk update per palette component instead of one render-state dirty
    // per segment; this keeps full rebuilds affordable at firehose feed rates.
    TArray<TArray<FTransform>> PerPalette;
    TArray<TArray<float>> PerPaletteSpawnTimes;
    TArray<TArray<float>> PerPaletteBrightness;
    PerPalette.SetNum(PaletteMeshes.Num());
    PerPaletteSpawnTimes.SetNum(PaletteMeshes.Num());
    PerPaletteBrightness.SetNum(PaletteMeshes.Num());
    for (const FRenderedGeoArc& Arc : ActiveArcs)
    {
        const int32 PaletteIndex = PaletteMeshes.IsValidIndex(Arc.PaletteIndex) ? Arc.PaletteIndex : PaletteMeshes.Num() - 1;
        AppendArcTransforms(PerPalette[PaletteIndex], Arc.Message, ArcThickness);
        for (int32 Segment = 0; Segment < SegmentsPerArc; ++Segment)
        {
            PerPaletteSpawnTimes[PaletteIndex].Add(static_cast<float>(Arc.SpawnTimeSeconds));
            PerPaletteBrightness[PaletteIndex].Add(SegmentBrightness(Arc, Segment));
        }
    }
    const float InverseLifetime = LifetimeSeconds > 0.0 ? static_cast<float>(1.0 / LifetimeSeconds) : 0.0f;
    for (int32 Index = 0; Index < PaletteMeshes.Num(); ++Index)
    {
        UInstancedStaticMeshComponent* Instances = PaletteMeshes[Index];
        Instances->ClearInstances();
        const TArray<int32> Added = Instances->AddInstances(PerPalette[Index], true, true);
        for (int32 AddedIndex = 0; AddedIndex < Added.Num(); ++AddedIndex)
        {
            Instances->SetCustomDataValue(Added[AddedIndex], 0, PerPaletteSpawnTimes[Index][AddedIndex], false);
            Instances->SetCustomDataValue(Added[AddedIndex], 1, InverseLifetime, false);
            Instances->SetCustomDataValue(Added[AddedIndex], 2, PerPaletteBrightness[Index][AddedIndex], false);
        }
        Instances->MarkRenderStateDirty();
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
    for (const FRenderedGeoArc& Arc : ActiveArcs)
    {
        const int32 PaletteIndex = Counts.IsValidIndex(Arc.PaletteIndex) ? Arc.PaletteIndex : Counts.Num() - 1;
        if (Counts.IsValidIndex(PaletteIndex)) ++Counts[PaletteIndex];
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
