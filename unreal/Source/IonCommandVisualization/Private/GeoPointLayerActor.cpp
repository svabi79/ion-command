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

    constexpr int32 MarkerCustomFloats = 7;
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

void AGeoPointLayerActor::AppendCustomData(TArray<float>& Out, const FRenderedGeoPoint& Point)
{
    Out.Add(Point.IconIndex);
    Out.Add(Point.Color.R);
    Out.Add(Point.Color.G);
    Out.Add(Point.Color.B);
    Out.Add(static_cast<float>(Point.Location.X));
    Out.Add(static_cast<float>(Point.Location.Y));
    Out.Add(static_cast<float>(Point.Location.Z));
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
    // surface events keep the small readability offset.
    const double AltitudeUnits = FMath::Max(Position.AltitudeMeters, 0.0) / 6371000.0 * GlobeRadius;
    const FVector Location = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Position.Latitude, Position.Longitude) * (GlobeRadius + 8.0 + AltitudeUnits);
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
    const bool bMoves = Position.AltitudeMeters > 1000.0;
    if (int32* ExistingIndex = EntityToPoint.Find(EntityKey))
    {
        FRenderedGeoPoint& Point = ActivePoints[*ExistingIndex];
        Point.LastSeenSeconds = NowSeconds;
        // Moving markers (satellites) need their instance transform updated,
        // not just the bookkeeping refresh static stations get.
        if (bMoves && !Point.Location.Equals(Location, 1.0))
        {
            bNeedsRebuild = true;
        }
        Point.Location = Location;
        if (ExpireAt > 0.0) Point.ExpireAtSeconds = ExpireAt;
        Point.Scale = PointScale;
        // Tooltip data follows the latest sighting (a climbing aircraft's
        // flight level, a station's newest report).
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
    Point.LastSeenSeconds = NowSeconds;
    Point.ExpireAtSeconds = ExpireAt;
    Point.Scale = PointScale;
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

void AGeoPointLayerActor::SetDomainVisible(const FString& Domain, bool bVisible)
{
    const bool bChanged = bVisible ? HiddenDomains.Remove(Domain) > 0 : !HiddenDomains.Contains(Domain);
    if (!bVisible) HiddenDomains.Add(Domain);
    if (bChanged) bNeedsRebuild = true;
}

const FRenderedGeoPoint* AGeoPointLayerActor::FindNearestToRay(const FVector& RayOrigin, const FVector& RayDirection, double MaxDistance) const
{
    const FRenderedGeoPoint* Best = nullptr;
    double BestDistance = MaxDistance;
    double BestAlong = TNumericLimits<double>::Max();
    for (const FRenderedGeoPoint& Point : ActivePoints)
    {
        if (!IsDomainVisible(Point.Domain)) continue;
        const FVector ToPoint = Point.Location - RayOrigin;
        const double Along = FVector::DotProduct(ToPoint, RayDirection);
        if (Along <= 0.0) continue;
        const double Distance = FVector::Dist(RayOrigin + RayDirection * Along, Point.Location);
        // Prefer the closest lateral hit; among near-ties take the marker
        // nearer the camera (the one visually in front).
        if (Distance < BestDistance || (Distance < BestDistance + 2.0 && Along < BestAlong))
        {
            Best = &Point;
            BestDistance = Distance;
            BestAlong = Along;
        }
    }
    return Best;
}

void AGeoPointLayerActor::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    if (bNeedsRebuild)
    {
        bNeedsRebuild = false;
        RebuildInstances();
    }
    const double NowSeconds = GetWorld() ? GetWorld()->GetTimeSeconds() : 0.0;
    if (NowSeconds - LastExpiryCheck > 5.0)
    {
        LastExpiryCheck = NowSeconds;
        auto Expired = [this, NowSeconds](const FRenderedGeoPoint& Point)
        {
            if (Point.ExpireAtSeconds > 0.0) return NowSeconds > Point.ExpireAtSeconds;
            return NowSeconds - Point.LastSeenSeconds > (Point.bObservation ? ObservationLifetimeSeconds : MarkerLifetimeSeconds);
        };
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
    // from closest approach to the default orbit, where markers are 1:1.
    const double Target = FMath::Clamp((CameraDistance - GlobeRadius) / 2400.0, 0.22, 1.15);
    if (FMath::Abs(Target - CurrentZoomFactor) / CurrentZoomFactor < 0.08) return;
    CurrentZoomFactor = Target;
    RebuildInstances();
}

void AGeoPointLayerActor::RebuildInstances()
{
    TArray<FTransform> Transforms;
    TArray<const FRenderedGeoPoint*> Rendered;
    Transforms.Reserve(ActivePoints.Num());
    Rendered.Reserve(ActivePoints.Num());
    for (const FRenderedGeoPoint& Point : ActivePoints)
    {
        if (!IsDomainVisible(Point.Domain)) continue;
        const FVector Scale(MarkerScale * CurrentZoomFactor * Point.Scale);
        Transforms.Emplace(FQuat::Identity, Point.Location, Scale);
        Rendered.Add(&Point);
    }
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
