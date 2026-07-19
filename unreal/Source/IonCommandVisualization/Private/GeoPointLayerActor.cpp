#include "GeoPointLayerActor.h"

#include "Components/InstancedStaticMeshComponent.h"
#include "Components/SceneComponent.h"
#include "GeoDataSubsystem.h"
#include "GeoLayerSubsystem.h"
#include "GeoMathLibrary.h"
#include "Materials/MaterialInstanceDynamic.h"
#include "UObject/ConstructorHelpers.h"

AGeoPointLayerActor::AGeoPointLayerActor()
{
    PrimaryActorTick.bCanEverTick = true;
    PrimaryActorTick.TickInterval = 0.25f;
    SceneRoot = CreateDefaultSubobject<USceneComponent>(TEXT("Root")); SetRootComponent(SceneRoot);
    static ConstructorHelpers::FObjectFinder<UStaticMesh> SphereMesh(TEXT("/Engine/BasicShapes/Sphere.Sphere"));
    EntityInstances = CreateDefaultSubobject<UInstancedStaticMeshComponent>(TEXT("EntityPoints"));
    EntityInstances->SetupAttachment(SceneRoot); EntityInstances->SetStaticMesh(SphereMesh.Object); EntityInstances->SetCollisionEnabled(ECollisionEnabled::NoCollision); EntityInstances->SetCastShadow(false);
    ObservationInstances = CreateDefaultSubobject<UInstancedStaticMeshComponent>(TEXT("ObservationPoints"));
    ObservationInstances->SetupAttachment(SceneRoot); ObservationInstances->SetStaticMesh(SphereMesh.Object); ObservationInstances->SetCollisionEnabled(ECollisionEnabled::NoCollision); ObservationInstances->SetCastShadow(false);
    if (UMaterialInterface* EntityMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Point_Entity.MI_Point_Entity"))) EntityInstances->SetMaterial(0, EntityMaterial);
    if (UMaterialInterface* ObservationMaterial = LoadObject<UMaterialInterface>(nullptr, TEXT("/Game/ION/Materials/MI_Point_Observation.MI_Point_Observation"))) ObservationInstances->SetMaterial(0, ObservationMaterial);
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
    EntityInstances->ClearInstances();
    ObservationInstances->ClearInstances();
    constexpr int32 PreviewPointCount = 180;
    for (int32 Index = 0; Index < PreviewPointCount; ++Index)
    {
        const double Longitude = FMath::Fmod(Index * 137.507764, 360.0) - 180.0;
        const double Latitude = FMath::Sin(Index * 1.13) * 70.0;
        const FVector Location = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Latitude, Longitude) * (GlobeRadius + 12.0);
        UInstancedStaticMeshComponent* Instances = Index % 7 == 0 ? ObservationInstances : EntityInstances;
        const double Scale = Index % 7 == 0 ? 0.13 : 0.075;
        Instances->AddInstance(FTransform(FQuat::Identity, Location, FVector(Scale)), true);
    }
}

void AGeoPointLayerActor::BeginPlay()
{
    Super::BeginPlay();
    Reset();
    if (UMaterialInstanceDynamic* Material = EntityInstances->CreateAndSetMaterialInstanceDynamic(0)) Material->SetVectorParameterValue(TEXT("Color"), FLinearColor(0.0f, 0.9f, 1.0f));
    if (UMaterialInstanceDynamic* Material = ObservationInstances->CreateAndSetMaterialInstanceDynamic(0)) Material->SetVectorParameterValue(TEXT("Color"), FLinearColor(1.0f, 0.25f, 0.02f));
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
    Point.Title = Message.Properties.FindRef(TEXT("display.title"));
    if (Point.Title.IsEmpty()) Point.Title = Message.Properties.FindRef(TEXT("callsign"));
    if (Point.Title.IsEmpty()) Point.Title = Message.SemanticType;
    Point.Primary = Message.Properties.FindRef(TEXT("display.primary"));
    Point.Secondary = Message.Properties.FindRef(TEXT("display.secondary"));
    EntityToPoint.Add(EntityKey, ActivePoints.Num() - 1);
    if (!bNeedsRebuild && IsDomainVisible(Point.Domain))
    {
        UInstancedStaticMeshComponent* Instances = Point.bObservation ? ObservationInstances : EntityInstances;
        Instances->AddInstance(FTransform(FQuat::Identity, Location, FVector(MarkerScale * CurrentZoomFactor * PointScale)), true);
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
    TArray<FTransform> Entities;
    TArray<FTransform> Observations;
    Entities.Reserve(ActivePoints.Num());
    for (const FRenderedGeoPoint& Point : ActivePoints)
    {
        if (!IsDomainVisible(Point.Domain)) continue;
        const FVector Scale(MarkerScale * CurrentZoomFactor * Point.Scale);
        (Point.bObservation ? Observations : Entities).Emplace(FQuat::Identity, Point.Location, Scale);
    }
    EntityInstances->ClearInstances();
    ObservationInstances->ClearInstances();
    EntityInstances->AddInstances(Entities, false);
    ObservationInstances->AddInstances(Observations, false);
    EntityInstances->MarkRenderStateDirty();
    ObservationInstances->MarkRenderStateDirty();
}

void AGeoPointLayerActor::Reset()
{
    ActivePoints.Reset();
    EntityToPoint.Reset();
    EntityInstances->ClearInstances();
    ObservationInstances->ClearInstances();
}
void AGeoPointLayerActor::OnMessageAccepted(const FGeoMessageEnvelope& Message) { Submit(Message); }
void AGeoPointLayerActor::OnLayerVisibilityChanged(const FString& LayerId, bool bVisible) { if (LayerId == TEXT("core.point-markers")) SetActorHiddenInGame(!bVisible); }
