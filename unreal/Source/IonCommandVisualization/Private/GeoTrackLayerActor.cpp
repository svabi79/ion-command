#include "GeoTrackLayerActor.h"

#include "Components/InstancedStaticMeshComponent.h"
#include "Components/SceneComponent.h"
#include "EngineUtils.h"
#include "GeoDataSubsystem.h"
#include "GeoMathLibrary.h"
#include "GeoPointLayerActor.h"
#include "UObject/ConstructorHelpers.h"

AGeoTrackLayerActor::AGeoTrackLayerActor()
{
    PrimaryActorTick.bCanEverTick = true;
    PrimaryActorTick.TickInterval = 0.25f;
    SceneRoot = CreateDefaultSubobject<USceneComponent>(TEXT("Root"));
    SetRootComponent(SceneRoot);
    // Same low-poly cube segment as AGeoArcLayerActor: at trail thickness the
    // cross-section is invisible under bloom, so the cheap BasicShapes cube
    // costs nothing extra over a "correct" cylinder (see that class for the
    // measured reasoning).
    static ConstructorHelpers::FObjectFinder<UStaticMesh> SegmentMesh(TEXT("/Engine/BasicShapes/Cube.Cube"));
    TrailInstances = CreateDefaultSubobject<UInstancedStaticMeshComponent>(TEXT("TrailInstances"));
    TrailInstances->SetupAttachment(SceneRoot);
    TrailInstances->SetStaticMesh(SegmentMesh.Object);
    TrailInstances->SetCollisionEnabled(ECollisionEnabled::NoCollision);
    TrailInstances->SetCastShadow(false);
    // Custom data 0/1 carry spawn time and 1/lifetime into the same
    // M_HolographicSignal GPU age-fade material the arc layer's palette
    // meshes use - no CPU fade. Custom data 2 (endpoint congestion dimming)
    // is intentionally left unset: with only two floats allocated it reads
    // as 0 in the shader, which the material's fallback maps to full
    // brightness, exactly like the arc layer's static endpoint/selection
    // meshes that also carry fewer than three custom floats.
    TrailInstances->SetNumCustomDataFloats(2);
    if (UMaterialInterface* TrailMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Track.MI_Track")))
    {
        TrailInstances->SetMaterial(0, TrailMaterial);
    }
}

void AGeoTrackLayerActor::BeginPlay()
{
    Super::BeginPlay();
    Reset();
    if (UGameInstance* GameInstance = GetGameInstance())
    {
        DataSubsystem = GameInstance->GetSubsystem<UGeoDataSubsystem>();
        if (DataSubsystem.IsValid())
        {
            DataSubsystem->OnMessageAccepted().AddUObject(this, &AGeoTrackLayerActor::OnMessageAccepted);
            DataSubsystem->OnDataReset().AddUObject(this, &AGeoTrackLayerActor::Reset);
            // Seed from whatever is already in the active window (e.g. after
            // a level restart while the collector kept running), the same
            // way AGeoArcLayerActor primes itself in BeginPlay.
            for (const FGeoMessageEnvelope& Message : DataSubsystem->GetActiveMessages())
            {
                OnMessageAccepted(Message);
            }
        }
    }
}

void AGeoTrackLayerActor::EndPlay(const EEndPlayReason::Type EndPlayReason)
{
    if (DataSubsystem.IsValid())
    {
        DataSubsystem->OnMessageAccepted().RemoveAll(this);
        DataSubsystem->OnDataReset().RemoveAll(this);
    }
    Super::EndPlay(EndPlayReason);
}

bool AGeoTrackLayerActor::Supports(const FGeoMessageEnvelope& Message) const
{
    // Same geometry contract as AGeoPointLayerActor: a trail is built by
    // watching successive Point sightings of one entity, not by consuming
    // the still-reserved Track geometry payload (nothing produces one today).
    return Message.Geometry.Type == EGeoGeometryType::Point && Message.Geometry.Positions.Num() == 1;
}

void AGeoTrackLayerActor::Submit(const FGeoMessageEnvelope& Message)
{
    if (!Supports(Message))
    {
        return;
    }
    // A trail needs the SAME stable identity across sightings. A one-shot
    // observation (no entity id - e.g. a lightning strike) can never repeat,
    // so by this definition it can never move; skip it before it ever
    // reaches the tracker instead of wasting a tracked-entity slot on a key
    // that will never be seen again.
    if (Message.EntityId.IsEmpty())
    {
        return;
    }

    const FGeoPosition& Position = Message.Geometry.Positions[0];
    // Mirrors GeoPointLayerActor's visual.altitudeScale parsing exactly so a
    // trail's altitude tracks the same domain-declared exaggeration as the
    // marker it follows.
    double DeclaredAltitudeScale = 1.0;
    const FString AltitudeScaleProperty = Message.Properties.FindRef(TEXT("visual.altitudeScale"));
    if (!AltitudeScaleProperty.IsEmpty())
    {
        DeclaredAltitudeScale = FMath::Clamp(FCString::Atod(*AltitudeScaleProperty), 1.0, 100.0);
    }

    Tracker.MaxTrackedEntities = MaxTrackedEntities;
    Tracker.MaxPointsPerTrail = MaxPointsPerTrail;
    Tracker.MovementThresholdMeters = MovementThresholdMeters;

    const double NowSeconds = GetWorld() ? GetWorld()->GetTimeSeconds() : 0.0;
    const EGeoTrackUpdateResult Result = Tracker.AddSighting(Message.EntityId, Position, DeclaredAltitudeScale, NowSeconds);
    if (Result == EGeoTrackUpdateResult::Moved)
    {
        bNeedsRebuild = true;
    }
}

void AGeoTrackLayerActor::Reset()
{
    Tracker.Reset();
    if (TrailInstances)
    {
        TrailInstances->ClearInstances();
    }
    bNeedsRebuild = false;
}

void AGeoTrackLayerActor::OnMessageAccepted(const FGeoMessageEnvelope& Message)
{
    Submit(Message);
}

bool AGeoTrackLayerActor::ResolveAltitudeExaggerationEnabled() const
{
    // Reads the operator's altitude-exaggeration toggle straight from the
    // point layer's own public getter instead of keeping a second,
    // independently-toggled copy of the same setting: trails must always
    // agree with the markers they follow. Falls back to the point layer's
    // own default (true) when none exists yet, e.g. in an isolated test.
    if (const UWorld* World = GetWorld())
    {
        for (TActorIterator<AGeoPointLayerActor> It(World); It; ++It)
        {
            return It->IsAltitudeExaggerationEnabled();
        }
    }
    return true;
}

FVector AGeoTrackLayerActor::ResolveRenderLocation(const FGeoPosition& Position, double DeclaredAltitudeScale, bool bAltitudeExaggeration) const
{
    // Mirrors GeoPointLayerActor's marker placement formula exactly (same
    // surface offset and altitude-to-globe-unit conversion) so a trail
    // visually hugs the marker it follows instead of floating at a different
    // height.
    const double ActiveScale = bAltitudeExaggeration ? DeclaredAltitudeScale : 1.0;
    const double AltitudeUnits = FMath::Max(Position.AltitudeMeters, 0.0) * ActiveScale / 6371000.0 * GlobeRadius;
    const FVector Radial = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Position.Latitude, Position.Longitude);
    return Radial * (GlobeRadius + 8.0 + AltitudeUnits);
}

void AGeoTrackLayerActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    const double NowSeconds = GetWorld() ? GetWorld()->GetTimeSeconds() : 0.0;

    if (NowSeconds - LastExpirySweepSeconds > 3.0)
    {
        LastExpirySweepSeconds = NowSeconds;
        // Small grace beyond the GPU fade window before geometry is actually
        // dropped, the same fade-then-sweep sequencing AGeoArcLayerActor uses
        // so nothing visible is yanked mid-fade.
        if (Tracker.RemoveExpiredPoints(NowSeconds, TrailPointLifetimeSeconds + 1.0))
        {
            bNeedsRebuild = true;
        }
    }

    if (bNeedsRebuild && NowSeconds - LastRebuildSeconds >= RebuildIntervalSeconds)
    {
        bNeedsRebuild = false;
        LastRebuildSeconds = NowSeconds;
        RebuildInstances();
    }
}

void AGeoTrackLayerActor::RebuildInstances()
{
    const bool bAltitudeExaggeration = ResolveAltitudeExaggerationEnabled();
    const int32 Legs = FMath::Max(1, SegmentsPerLeg);
    const float InverseLifetime = TrailPointLifetimeSeconds > 0.0 ? static_cast<float>(1.0 / TrailPointLifetimeSeconds) : 0.0f;

    TArray<FTransform> Transforms;
    TArray<float> SpawnTimes;
    // Reserve against the worst case so this never reallocates mid-build:
    // bounded by the two operator-facing caps times the subdivision count.
    const int32 Budget = FMath::Max(0, MaxTrackedEntities) * FMath::Max(0, MaxPointsPerTrail - 1) * Legs;
    Transforms.Reserve(Budget);
    SpawnTimes.Reserve(Budget);

    for (const TPair<FString, FGeoTrackedTrail>& Pair : Tracker.GetTrails())
    {
        const TArray<FGeoTrailPoint>& Points = Pair.Value.Points;
        if (Points.Num() < 2)
        {
            continue;
        }
        FVector Previous = ResolveRenderLocation(Points[0].Position, Points[0].DeclaredAltitudeScale, bAltitudeExaggeration);
        for (int32 PointIndex = 1; PointIndex < Points.Num(); ++PointIndex)
        {
            const FGeoTrailPoint& FromPoint = Points[PointIndex - 1];
            const FGeoTrailPoint& ToPoint = Points[PointIndex];
            // The leg leading into the newer point shares that point's
            // capture time, so it snaps back to full brightness the instant
            // a fresh sighting extends the trail, then fades toward the tail
            // as that sample ages - a comet effect entirely driven by the
            // existing GPU mechanism, no per-frame CPU work.
            const float SpawnSeconds = static_cast<float>(ToPoint.AddedSeconds);
            // Tail segments render thinner than the head. Purely a static,
            // CPU-computed cue that keeps a trail readable even at a moment
            // the GPU fade has not yet visibly separated head from tail; the
            // age fade supplies the actual animation.
            const double TaperAlpha = static_cast<double>(PointIndex) / FMath::Max(1, Points.Num() - 1);
            const double Thickness = TrailThickness * FMath::Lerp(0.4, 1.0, TaperAlpha);

            for (int32 Leg = 1; Leg <= Legs; ++Leg)
            {
                const double Alpha = static_cast<double>(Leg) / Legs;
                // Great-circle interpolation (not a straight chord) keeps the
                // trail hugging the globe surface even when two consecutive
                // samples are far apart, exactly like AGeoArcLayerActor's
                // propagation arcs use it for their path - the difference is
                // this leg follows the entity's own interpolated altitude
                // rather than an artificial signal-arc bulge, since a trail
                // is a real trajectory, not a presentation metaphor.
                const FGeoPosition Interpolated = UGeoMathLibrary::GreatCircleInterpolation(FromPoint.Position, ToPoint.Position, Alpha);
                const double InterpolatedScale = FMath::Lerp(FromPoint.DeclaredAltitudeScale, ToPoint.DeclaredAltitudeScale, Alpha);
                const FVector Current = ResolveRenderLocation(Interpolated, InterpolatedScale, bAltitudeExaggeration);
                const FVector Delta = Current - Previous;
                const double Length = Delta.Size();
                if (Length > UE_DOUBLE_SMALL_NUMBER)
                {
                    const FVector Midpoint = (Previous + Current) * 0.5;
                    const FRotator Rotation = FRotationMatrix::MakeFromZ(Delta.GetSafeNormal()).Rotator();
                    Transforms.Emplace(Rotation, Midpoint, FVector(Thickness, Thickness, Length / 100.0));
                    SpawnTimes.Add(SpawnSeconds);
                }
                Previous = Current;
            }
        }
    }

    TrailInstances->ClearInstances();
    const TArray<int32> Added = TrailInstances->AddInstances(Transforms, true);
    for (int32 Index = 0; Index < Added.Num(); ++Index)
    {
        TrailInstances->SetCustomDataValue(Added[Index], 0, SpawnTimes[Index], false);
        TrailInstances->SetCustomDataValue(Added[Index], 1, InverseLifetime, false);
    }
    TrailInstances->MarkRenderStateDirty();
}
