#include "GeoPointLayerActor.h"

#include "Components/InstancedStaticMeshComponent.h"
#include "Components/SceneComponent.h"
#include "GeoDataSubsystem.h"
#include "GeoLayerSubsystem.h"
#include "GeoMathLibrary.h"
#include "Materials/MaterialInstanceDynamic.h"
#include "UObject/ConstructorHelpers.h"

namespace
{
    struct FMarkerIconStyle
    {
        float AtlasIndex = 0.0f;
        FLinearColor Color = FLinearColor(0.0f, 0.82f, 1.0f);
    };

    // Generic pictogram vocabulary: domains declare their glyph through the
    // envelope's visual.icon property; the renderer only knows atlas tiles.
    const TMap<FString, FMarkerIconStyle>& IconStyles()
    {
        static const TMap<FString, FMarkerIconStyle> Styles = {
            {TEXT("signal"),     {1.0f, FLinearColor(0.00f, 0.85f, 1.00f)}},
            {TEXT("aircraft"),   {2.0f, FLinearColor(1.00f, 0.72f, 0.10f)}},
            {TEXT("satellite"),  {3.0f, FLinearColor(0.85f, 0.92f, 1.00f)}},
            {TEXT("lightning"),  {4.0f, FLinearColor(0.78f, 0.55f, 1.00f)}},
            {TEXT("sounding"),   {5.0f, FLinearColor(0.25f, 1.00f, 0.55f)}},
            {TEXT("earthquake"), {6.0f, FLinearColor(1.00f, 0.28f, 0.12f)}},
            // Airframe kinds share the aviation amber; the silhouette is
            // the discriminator.
            {TEXT("helicopter"), {8.0f, FLinearColor(1.00f, 0.72f, 0.10f)}},
            {TEXT("balloon"),    {9.0f, FLinearColor(1.00f, 0.60f, 0.25f)}},
            {TEXT("drone"),      {10.0f, FLinearColor(1.00f, 0.45f, 0.20f)}},
            {TEXT("glider"),     {11.0f, FLinearColor(1.00f, 0.85f, 0.35f)}},
        };
        return Styles;
    }

    // Replay compatibility only: recordings made before domains declared
    // visual.icon still get their glyph. New domains must send the property.
    const TMap<FString, FString>& DomainIconFallback()
    {
        static const TMap<FString, FString> Fallback = {
            {TEXT("hamradio"), TEXT("signal")},
            {TEXT("aviation"), TEXT("aircraft")},
            {TEXT("orbital"), TEXT("satellite")},
            {TEXT("weather"), TEXT("lightning")},
            {TEXT("ionosphere"), TEXT("sounding")},
            {TEXT("geophysics"), TEXT("earthquake")},
        };
        return Fallback;
    }

    constexpr int32 MarkerCustomFloats = 10;

    // World-space compass heading at a globe position, from the ANALYTIC
    // tangent frame of the pinned sphere convention
    // pos = (cosLat*sinLon, cosLat*cosLon, sinLat):
    //   East  = d(pos)/d(lon) normalised = (cosLon, -sinLon, 0)
    //   North = d(pos)/d(lat)            = (-sinLat*sinLon, -sinLat*cosLon, cosLat)
    // Both are exact unit vectors at every latitude, so heading stays valid
    // to the poles. The previous finite-difference East collapsed to zero
    // (SafeNormal tolerance) beyond ~86.7 deg and froze polar movers
    // (audit finding #9).
    FVector CompassHeadingWorld(double Latitude, double Longitude, double HeadingDegrees)
    {
        const double LatRad = FMath::DegreesToRadians(Latitude);
        const double LonRad = FMath::DegreesToRadians(Longitude);
        const double SinLat = FMath::Sin(LatRad);
        const double CosLat = FMath::Cos(LatRad);
        const double SinLon = FMath::Sin(LonRad);
        const double CosLon = FMath::Cos(LonRad);
        const FVector East(CosLon, -SinLon, 0.0);
        const FVector North(-SinLat * SinLon, -SinLat * CosLon, CosLat);
        const double Radians = FMath::DegreesToRadians(HeadingDegrees);
        return (North * FMath::Cos(Radians) + East * FMath::Sin(Radians)).GetSafeNormal();
    }
}

AGeoPointLayerActor::AGeoPointLayerActor()
{
    PrimaryActorTick.bCanEverTick = true;
    PrimaryActorTick.TickInterval = 0.25f;
    SceneRoot = CreateDefaultSubobject<USceneComponent>(TEXT("Root")); SetRootComponent(SceneRoot);
    static ConstructorHelpers::FObjectFinder<UStaticMesh> QuadMesh(TEXT("/Engine/BasicShapes/Plane.Plane"));
    MarkerInstances = CreateDefaultSubobject<UInstancedStaticMeshComponent>(TEXT("MarkerPoints"));
    MarkerInstances->SetupAttachment(SceneRoot); MarkerInstances->SetStaticMesh(QuadMesh.Object); MarkerInstances->SetCollisionEnabled(ECollisionEnabled::NoCollision); MarkerInstances->SetCastShadow(false);
    MarkerInstances->SetNumCustomDataFloats(MarkerCustomFloats);
    if (UMaterialInterface* IconMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/M_MarkerIcon.M_MarkerIcon"))) MarkerInstances->SetMaterial(0, IconMaterial);
}

bool AGeoPointLayerActor::IsExpired(const FRenderedGeoPoint& Point, double NowSeconds) const
{
    if (Point.ExpireAtSeconds > 0.0) return NowSeconds > Point.ExpireAtSeconds;
    return NowSeconds - Point.LastSeenSeconds > (Point.bObservation ? ObservationLifetimeSeconds : MarkerLifetimeSeconds);
}

void AGeoPointLayerActor::AppendCustomData(TArray<float>& Out, const FRenderedGeoPoint& Point)
{
    Out.Add(Point.IconIndex);
    Out.Add(Point.Color.R);
    Out.Add(Point.Color.G);
    Out.Add(Point.Color.B);
    // Billboard pivot: the position the instance is actually drawn at.
    Out.Add(static_cast<float>(Point.RenderedLocation.X));
    Out.Add(static_cast<float>(Point.RenderedLocation.Y));
    Out.Add(static_cast<float>(Point.RenderedLocation.Z));
    Out.Add(static_cast<float>(Point.HeadingWorld.X));
    Out.Add(static_cast<float>(Point.HeadingWorld.Y));
    Out.Add(static_cast<float>(Point.HeadingWorld.Z));
}

void AGeoPointLayerActor::OnConstruction(const FTransform& Transform)
{
    Super::OnConstruction(Transform);
    if (GetWorld() && !GetWorld()->IsGameWorld()) BuildEditorPreview();
}

void AGeoPointLayerActor::PostRegisterAllComponents()
{
    Super::PostRegisterAllComponents();
    if (GetWorld() && !GetWorld()->IsGameWorld()) BuildEditorPreview();
}

void AGeoPointLayerActor::BuildEditorPreview()
{
    MarkerInstances->ClearInstances();
    constexpr int32 PreviewPointCount = 180;
    for (int32 Index = 0; Index < PreviewPointCount; ++Index)
    {
        const double Longitude = FMath::Fmod(Index * 137.507764, 360.0) - 180.0;
        const double Latitude = FMath::Sin(Index * 1.13) * 70.0;
        const FVector Location = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Latitude, Longitude) * (GlobeRadius + 12.0);
        const int32 InstanceIndex = MarkerInstances->AddInstance(FTransform(FQuat::Identity, Location, FVector(0.1)), true);
        FRenderedGeoPoint Preview;
        Preview.IconIndex = static_cast<float>(Index % 8);
        Preview.Color = FLinearColor::White;
        Preview.Location = Location;
        Preview.RenderedLocation = Location;
        TArray<float> CustomData;
        AppendCustomData(CustomData, Preview);
        MarkerInstances->SetCustomData(InstanceIndex, CustomData, false);
    }
    MarkerInstances->MarkRenderStateDirty();
}

void AGeoPointLayerActor::BeginPlay()
{
    Super::BeginPlay();
    Reset();
    if (UGameInstance* GameInstance = GetGameInstance())
    {
        DataSubsystem = GameInstance->GetSubsystem<UGeoDataSubsystem>();
        if (DataSubsystem.IsValid()) { DataSubsystem->OnMessageAccepted().AddUObject(this, &AGeoPointLayerActor::OnMessageAccepted); DataSubsystem->OnDataReset().AddUObject(this, &AGeoPointLayerActor::Reset); }
        if (UGeoLayerSubsystem* LayerSubsystem = GameInstance->GetSubsystem<UGeoLayerSubsystem>())
        {
            FGeoLayerManifest Manifest; Manifest.LayerId = TEXT("core.point-markers"); Manifest.DisplayName = TEXT("Geospatial Point Markers"); Manifest.GeometryTypes = {EGeoGeometryType::Point}; Manifest.bSupportsAggregation = true; LayerSubsystem->RegisterLayer(Manifest); LayerSubsystem->OnLayerVisibilityChanged().AddUObject(this, &AGeoPointLayerActor::OnLayerVisibilityChanged); SetActorHiddenInGame(!LayerSubsystem->IsLayerVisible(Manifest.LayerId));
        }
    }
}

void AGeoPointLayerActor::EndPlay(const EEndPlayReason::Type EndPlayReason) { if (DataSubsystem.IsValid()) { DataSubsystem->OnMessageAccepted().RemoveAll(this); DataSubsystem->OnDataReset().RemoveAll(this); } if (UGameInstance* GameInstance = GetGameInstance()) if (UGeoLayerSubsystem* LayerSubsystem = GameInstance->GetSubsystem<UGeoLayerSubsystem>()) LayerSubsystem->OnLayerVisibilityChanged().RemoveAll(this); Super::EndPlay(EndPlayReason); }
bool AGeoPointLayerActor::Supports(const FGeoMessageEnvelope& Message) const { return Message.Geometry.Type == EGeoGeometryType::Point && Message.Geometry.Positions.Num() == 1; }
void AGeoPointLayerActor::Submit(const FGeoMessageEnvelope& Message)
{
    if (!Supports(Message)) return;
    // Markers are stable per entity: a new sighting refreshes the existing
    // marker instead of stacking another instance on top of it. Messages
    // without an entity id fall back to their message id (one-shot events).
    const FString& EntityKey = !Message.EntityId.IsEmpty() ? Message.EntityId : Message.MessageId;
    const double NowSeconds = GetWorld() ? GetWorld()->GetTimeSeconds() : 0.0;
    const FGeoPosition& Position = Message.Geometry.Positions[0];
    // Altitude lifts markers off the surface at globe scale (satellites);
    // surface events keep the small readability offset. Domains whose true
    // altitude is imperceptible at globe scale (aircraft: 10 km on a
    // 6371 km sphere) declare a visual exaggeration factor.
    double DeclaredAltitudeScale = 1.0;
    const FString AltitudeScaleProperty = Message.Properties.FindRef(TEXT("visual.altitudeScale"));
    if (!AltitudeScaleProperty.IsEmpty())
    {
        DeclaredAltitudeScale = FMath::Clamp(FCString::Atod(*AltitudeScaleProperty), 1.0, 100.0);
    }
    const double ActiveScale = bAltitudeExaggeration ? DeclaredAltitudeScale : 1.0;
    const double AltitudeUnits = FMath::Max(Position.AltitudeMeters, 0.0) * ActiveScale / 6371000.0 * GlobeRadius;
    const FVector Radial = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Position.Latitude, Position.Longitude);
    const FVector Location = Radial * (GlobeRadius + 8.0 + AltitudeUnits);
    // validUntil bounds the marker's visual lifetime when the domain set one.
    double ExpireAt = 0.0;
    if (Message.Time.bHasValidUntil)
    {
        ExpireAt = NowSeconds + FMath::Clamp((Message.Time.ValidUntilUtc - FDateTime::UtcNow()).GetTotalSeconds(), 5.0, 7200.0);
    }
    float PointScale = 1.0f;
    const FString ScaleProperty = Message.Properties.FindRef(TEXT("visual.markerScale"));
    if (!ScaleProperty.IsEmpty())
    {
        PointScale = FMath::Clamp(FCString::Atof(*ScaleProperty), 0.3f, 5.0f);
    }
    const bool bMessageEmergency = !Message.Properties.FindRef(TEXT("visual.emergency")).IsEmpty();
    const bool bMessageOnGround = Message.Properties.FindRef(TEXT("onGround")) == TEXT("true");
    // Kinematics: compass heading and ground speed, converted into a world
    // tangent vector and globe units per second for glyph orientation and
    // dead reckoning.
    FVector HeadingWorld = FVector::ZeroVector;
    double SpeedUnitsPerSecond = 0.0;
    const FString HeadingProperty = Message.Properties.FindRef(TEXT("visual.headingDeg"));
    if (!HeadingProperty.IsEmpty())
    {
        HeadingWorld = CompassHeadingWorld(Position.Latitude, Position.Longitude, FCString::Atod(*HeadingProperty));
        const FString SpeedProperty = Message.Properties.FindRef(TEXT("visual.speedMps"));
        if (!SpeedProperty.IsEmpty())
        {
            SpeedUnitsPerSecond = FCString::Atod(*SpeedProperty) / 6371000.0 * GlobeRadius;
        }
    }
    if (int32* ExistingIndex = EntityToPoint.Find(EntityKey))
    {
        FRenderedGeoPoint& Point = ActivePoints[*ExistingIndex];
        Point.LastSeenSeconds = NowSeconds;
        // Any marker whose rendered position has drifted past the tolerance
        // needs a rebuild - the altitude proxy that used to gate this froze
        // low/ground movers whose feed omitted speed (audit finding #6). The
        // tolerance alone already suppresses stationary jitter.
        if (!Point.RenderedLocation.Equals(Location, MovementTolerance))
        {
            bMovementDirty = true;
        }
        Point.Location = Location;
        Point.RadialDirection = Radial;
        Point.AltitudeMeters = Position.AltitudeMeters;
        Point.DeclaredAltitudeScale = DeclaredAltitudeScale;
        Point.bOnGround = bMessageOnGround;
        Point.HeadingWorld = HeadingWorld;
        Point.SpeedUnitsPerSecond = SpeedUnitsPerSecond;
        if (ExpireAt > 0.0) Point.ExpireAtSeconds = ExpireAt;
        Point.Scale = PointScale;
        // Icon and tint follow the latest sighting: a later source may know
        // the airframe kind, and an emergency squawk must turn red NOW.
        const FString FreshTint = Message.Properties.FindRef(TEXT("visual.tint"));
        FString FreshIcon = Message.Properties.FindRef(TEXT("visual.icon"));
        if (FreshIcon.IsEmpty()) FreshIcon = DomainIconFallback().FindRef(Message.Domain);
        if (const FMarkerIconStyle* FreshStyle = IconStyles().Find(FreshIcon))
        {
            if (!FMath::IsNearlyEqual(Point.IconIndex, FreshStyle->AtlasIndex))
            {
                Point.IconIndex = FreshStyle->AtlasIndex;
                bNeedsRebuild = true;
            }
            Point.Color = FreshStyle->Color;
        }
        if (!FreshTint.IsEmpty())
        {
            TArray<FString> Parts;
            FreshTint.ParseIntoArray(Parts, TEXT(","));
            if (Parts.Num() == 3)
            {
                Point.Color = FLinearColor(FCString::Atof(*Parts[0]), FCString::Atof(*Parts[1]), FCString::Atof(*Parts[2]));
                bNeedsRebuild = true;
            }
        }
        // Emergency is sticky: a later non-emergency sighting (typically
        // OpenSky's null squawk) must not clear the alarm the normal tint
        // block just tried to downgrade (audit finding #2).
        if (bMessageEmergency) Point.bEmergency = true;
        if (Point.bEmergency)
        {
            Point.Color = FLinearColor(1.0f, 0.15f, 0.1f);
            Point.Scale = FMath::Max(Point.Scale, 2.0f);
            bNeedsRebuild = true;
        }
        // Tooltip data follows the latest sighting (a climbing aircraft's
        // flight level, a station's newest report). Title too - so a plane
        // that starts squawking mid-flight gets the alarm - but an emergency
        // title is never downgraded by a later non-emergency message.
        const FString NewTitle = Message.Properties.FindRef(TEXT("display.title"));
        if (!NewTitle.IsEmpty() && (bMessageEmergency || !Point.bEmergency)) Point.Title = NewTitle;
        const FString NewPrimary = Message.Properties.FindRef(TEXT("display.primary"));
        if (!NewPrimary.IsEmpty()) Point.Primary = NewPrimary;
        const FString NewSecondary = Message.Properties.FindRef(TEXT("display.secondary"));
        if (!NewSecondary.IsEmpty()) Point.Secondary = NewSecondary;
        return;
    }
    if (ActivePoints.Num() >= MaxVisiblePoints)
    {
        // Trim the oldest twentieth instead of wiping the whole class.
        const int32 RemoveCount = FMath::Max(1, MaxVisiblePoints / 20);
        ActivePoints.Sort([](const FRenderedGeoPoint& A, const FRenderedGeoPoint& B) { return A.LastSeenSeconds < B.LastSeenSeconds; });
        ActivePoints.RemoveAt(0, RemoveCount, EAllowShrinking::No);
        EntityToPoint.Reset();
        for (int32 Index = 0; Index < ActivePoints.Num(); ++Index) EntityToPoint.Add(ActivePoints[Index].EntityKey, Index);
        bNeedsRebuild = true;
    }
    FRenderedGeoPoint& Point = ActivePoints.AddDefaulted_GetRef();
    Point.EntityKey = EntityKey;
    Point.Location = Location;
    Point.RenderedLocation = Location;
    Point.RadialDirection = Radial;
    Point.AltitudeMeters = Position.AltitudeMeters;
    Point.DeclaredAltitudeScale = DeclaredAltitudeScale;
    Point.bOnGround = bMessageOnGround;
    Point.HeadingWorld = HeadingWorld;
    Point.SpeedUnitsPerSecond = SpeedUnitsPerSecond;
    // A lone kinematic marker added into an otherwise-quiet, static-camera
    // scene must start the coast cadence itself; otherwise the flag is only
    // ever latched inside RebuildInstances and it never begins (finding #14).
    if (SpeedUnitsPerSecond > 0.0) bHasKinematicPoints = true;
    Point.LastSeenSeconds = NowSeconds;
    Point.ExpireAtSeconds = ExpireAt;
    Point.Scale = PointScale;
    Point.bEmergency = bMessageEmergency;
    Point.bObservation = Message.MessageType != EGeoMessageType::Entity;
    Point.Domain = Message.Domain;
    // Pictogram: the domain's declared glyph, a replay-era fallback by
    // domain name, else the plain dot in the classic entity/observation hue.
    FString IconName = Message.Properties.FindRef(TEXT("visual.icon"));
    if (IconName.IsEmpty()) IconName = DomainIconFallback().FindRef(Message.Domain);
    if (const FMarkerIconStyle* Style = IconStyles().Find(IconName))
    {
        Point.IconIndex = Style->AtlasIndex;
        Point.Color = Style->Color;
    }
    else
    {
        Point.IconIndex = 0.0f;
        Point.Color = Point.bObservation ? FLinearColor(1.0f, 0.32f, 0.05f) : FLinearColor(0.0f, 0.82f, 1.0f);
    }
    // Explicit tint override (e.g. emergency-squawk aircraft turn red).
    const FString TintProperty = Message.Properties.FindRef(TEXT("visual.tint"));
    if (!TintProperty.IsEmpty())
    {
        TArray<FString> Parts;
        TintProperty.ParseIntoArray(Parts, TEXT(","));
        if (Parts.Num() == 3)
        {
            Point.Color = FLinearColor(FCString::Atof(*Parts[0]), FCString::Atof(*Parts[1]), FCString::Atof(*Parts[2]));
        }
    }
    Point.Title = Message.Properties.FindRef(TEXT("display.title"));
    if (Point.Title.IsEmpty()) Point.Title = Message.Properties.FindRef(TEXT("callsign"));
    if (Point.Title.IsEmpty()) Point.Title = Message.SemanticType;
    Point.Primary = Message.Properties.FindRef(TEXT("display.primary"));
    Point.Secondary = Message.Properties.FindRef(TEXT("display.secondary"));
    EntityToPoint.Add(EntityKey, ActivePoints.Num() - 1);
    if (!bNeedsRebuild && IsDomainVisible(Point.Domain))
    {
        const int32 InstanceIndex = MarkerInstances->AddInstance(FTransform(FQuat::Identity, Location, FVector(MarkerScale * CurrentZoomFactor * PointScale)), true);
        TArray<float> CustomData;
        AppendCustomData(CustomData, Point);
        MarkerInstances->SetCustomData(InstanceIndex, CustomData, true);
    }
}

void AGeoPointLayerActor::GetPresentDomains(TArray<FString>& OutDomains) const
{
    TSet<FString> Domains;
    for (const FRenderedGeoPoint& Point : ActivePoints)
    {
        if (!Point.Domain.IsEmpty()) Domains.Add(Point.Domain);
    }
    for (const FString& Hidden : HiddenDomains) Domains.Add(Hidden);
    OutDomains = Domains.Array();
    OutDomains.Sort();
}

void AGeoPointLayerActor::SetAltitudeExaggerationEnabled(bool bEnabled)
{
    if (bAltitudeExaggeration == bEnabled) return;
    bAltitudeExaggeration = bEnabled;
    // Recompute every marker's radial lift from the stored true altitude.
    for (FRenderedGeoPoint& Point : ActivePoints)
    {
        if (Point.RadialDirection.IsNearlyZero()) continue;
        const double Scale = bAltitudeExaggeration ? Point.DeclaredAltitudeScale : 1.0;
        const double AltitudeUnits = FMath::Max(Point.AltitudeMeters, 0.0) * Scale / 6371000.0 * GlobeRadius;
        Point.Location = Point.RadialDirection * (GlobeRadius + 8.0 + AltitudeUnits);
    }
    bNeedsRebuild = true;
}

bool AGeoPointLayerActor::IsAircraftFiltered(const FRenderedGeoPoint& Point) const
{
    // Only aviation markers are affected; satellites/ham/etc. never filter.
    if (Point.Domain != TEXT("aviation")) return false;
    if (Point.bOnGround) return !bShowGroundAircraft;
    return Point.AltitudeMeters < MinAircraftAltitudeMeters;
}

void AGeoPointLayerActor::SetMinAircraftAltitudeMeters(double Meters)
{
    Meters = FMath::Max(0.0, Meters);
    if (FMath::IsNearlyEqual(MinAircraftAltitudeMeters, Meters)) return;
    MinAircraftAltitudeMeters = Meters;
    bNeedsRebuild = true;
}

void AGeoPointLayerActor::SetShowGroundAircraft(bool bShow)
{
    if (bShowGroundAircraft == bShow) return;
    bShowGroundAircraft = bShow;
    bNeedsRebuild = true;
}

void AGeoPointLayerActor::SetMarkerLifetimeSeconds(double Seconds)
{
    MarkerLifetimeSeconds = FMath::Clamp(Seconds, 15.0, 7200.0);
    bNeedsRebuild = true;
}

void AGeoPointLayerActor::SetDomainVisible(const FString& Domain, bool bVisible)
{
    const bool bChanged = bVisible ? HiddenDomains.Remove(Domain) > 0 : !HiddenDomains.Contains(Domain);
    if (!bVisible) HiddenDomains.Add(Domain);
    if (bChanged) bNeedsRebuild = true;
}

const FRenderedGeoPoint* AGeoPointLayerActor::FindNearestToRay(const FVector& RayOrigin, const FVector& RayDirection, double MaxDistance) const
{
    // Never report markers the operator cannot see: a tooltip on an invisible
    // marker reads as "marker is missing" and sends debugging the wrong way.
    if (IsHidden()) return nullptr;
    const double NowSeconds = GetWorld() ? GetWorld()->GetTimeSeconds() : 0.0;
    const FRenderedGeoPoint* Best = nullptr;
    // MinLateral only ever decreases so the near-tie window is measured from
    // the genuinely closest hit; the old ratcheting BestDistance could raise
    // the reference and let a laterally-farther marker win (finding #17).
    double MinLateral = MaxDistance;
    double BestAlong = TNumericLimits<double>::Max();
    for (const FRenderedGeoPoint& Point : ActivePoints)
    {
        if (!IsDomainVisible(Point.Domain)) continue;
        // Skip expired markers awaiting the batched cleanup sweep.
        if (IsExpired(Point, NowSeconds)) continue;
        if (IsAircraftFiltered(Point)) continue;
        const FVector ToPoint = Point.RenderedLocation - RayOrigin;
        const double Along = FVector::DotProduct(ToPoint, RayDirection);
        if (Along <= 0.0) continue;
        const double Distance = FVector::Dist(RayOrigin + RayDirection * Along, Point.RenderedLocation);
        if (Distance > MinLateral + 2.0) continue;
        if (Distance < MinLateral) MinLateral = Distance;
        // Among markers within the closest hit's lateral window, take the one
        // nearest the camera (visually in front).
        if (Best == nullptr || Along < BestAlong)
        {
            Best = &Point;
            BestAlong = Along;
        }
    }
    return Best;
}

void AGeoPointLayerActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    const double NowSeconds = GetWorld() ? GetWorld()->GetTimeSeconds() : 0.0;
    // Kinematic markers keep the cadence running even without fresh
    // sightings: dead reckoning needs periodic rebuilds to glide.
    if (bNeedsRebuild || ((bMovementDirty || bHasKinematicPoints) && NowSeconds - LastMovementRebuild >= MovementRebuildSeconds))
    {
        bNeedsRebuild = false;
        RebuildInstances();
    }
    if (NowSeconds - LastExpiryCheck > 5.0)
    {
        LastExpiryCheck = NowSeconds;
        auto Expired = [this, NowSeconds](const FRenderedGeoPoint& Point) { return IsExpired(Point, NowSeconds); };
        int32 ExpiredCount = 0;
        for (const FRenderedGeoPoint& Point : ActivePoints) if (Expired(Point)) ++ExpiredCount;
        if (ExpiredCount > FMath::Max(8, ActivePoints.Num() / 20))
        {
            ActivePoints.RemoveAll(Expired);
            EntityToPoint.Reset();
            for (int32 Index = 0; Index < ActivePoints.Num(); ++Index) EntityToPoint.Add(ActivePoints[Index].EntityKey, Index);
            RebuildInstances();
        }
    }
    const APlayerController* Player = GetWorld() ? GetWorld()->GetFirstPlayerController() : nullptr;
    if (!Player || !Player->PlayerCameraManager) return;
    const double CameraDistance = Player->PlayerCameraManager->GetCameraLocation().Length();
    // Screen size tracks the distance to the near surface; 2400 is the span
    // from closest approach to the default orbit, where markers are 1:1. The
    // old 0.22 floor was tuned for a closest approach of 450 units; the camera
    // now comes down to 40, so the factor keeps shrinking through that range
    // (effective minimum 0.0167 at arm 1040 - the 0.012 floor is a safety
    // margin below it, not a target) and markers hold their screen size
    // instead of ballooning.
    const double Target = FMath::Clamp((CameraDistance - GlobeRadius) / 2400.0, 0.012, 1.15);
    if (FMath::Abs(Target - CurrentZoomFactor) / CurrentZoomFactor < 0.08) return;
    CurrentZoomFactor = Target;
    RebuildInstances();
}

void AGeoPointLayerActor::RebuildInstances()
{
    const double NowSeconds = GetWorld() ? GetWorld()->GetTimeSeconds() : 0.0;
    TArray<FTransform> Transforms;
    TArray<const FRenderedGeoPoint*> Rendered;
    Transforms.Reserve(ActivePoints.Num());
    Rendered.Reserve(ActivePoints.Num());
    bHasKinematicPoints = false;
    for (FRenderedGeoPoint& Point : ActivePoints)
    {
        // Dead reckoning: glide the marker along its heading between
        // sightings (capped so a stalled feed cannot fly it off course);
        // each fresh sighting snaps the base position back to truth.
        Point.RenderedLocation = Point.Location;
        if (Point.SpeedUnitsPerSecond > 0.0)
        {
            // Cap covers the slow global-snapshot cycle (15 min polls).
            const double CoastSeconds = FMath::Clamp(NowSeconds - Point.LastSeenSeconds, 0.0, 1200.0);
            Point.RenderedLocation += Point.HeadingWorld * (Point.SpeedUnitsPerSecond * CoastSeconds);
        }
        if (!IsDomainVisible(Point.Domain)) continue;
        // Do not draw markers already past their lifetime but still waiting
        // for the batched sweep: they rendered as glyph-less ghosts that the
        // hover pick nonetheless reported (audit finding #4).
        if (IsExpired(Point, NowSeconds)) continue;
        if (IsAircraftFiltered(Point)) continue;
        if (Point.SpeedUnitsPerSecond > 0.0) bHasKinematicPoints = true;
        const FVector Scale(MarkerScale * CurrentZoomFactor * Point.Scale);
        Transforms.Emplace(FQuat::Identity, Point.RenderedLocation, Scale);
        Rendered.Add(&Point);
    }
    bMovementDirty = false;
    LastMovementRebuild = NowSeconds;
    MarkerInstances->ClearInstances();
    MarkerInstances->AddInstances(Transforms, false);
    for (int32 Index = 0; Index < Rendered.Num(); ++Index)
    {
        TArray<float> CustomData;
        AppendCustomData(CustomData, *Rendered[Index]);
        MarkerInstances->SetCustomData(Index, CustomData, false);
    }
    MarkerInstances->MarkRenderStateDirty();
}

void AGeoPointLayerActor::Reset()
{
    ActivePoints.Reset();
    EntityToPoint.Reset();
    MarkerInstances->ClearInstances();
}
void AGeoPointLayerActor::OnMessageAccepted(const FGeoMessageEnvelope& Message) { Submit(Message); }
void AGeoPointLayerActor::OnLayerVisibilityChanged(const FString& LayerId, bool bVisible) { if (LayerId == TEXT("core.point-markers")) SetActorHiddenInGame(!bVisible); }
