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
    UInstancedStaticMeshComponent* Instances = Message.MessageType == EGeoMessageType::Entity ? EntityInstances : ObservationInstances;
    if (Instances->GetInstanceCount() >= MaxVisiblePoints) Instances->ClearInstances();
    const FGeoPosition& Position = Message.Geometry.Positions[0];
    const FVector Location = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Position.Latitude, Position.Longitude) * (GlobeRadius + 8.0);
    Instances->AddInstance(FTransform(FQuat::Identity, Location, FVector(0.14)), true);
}
void AGeoPointLayerActor::Reset() { EntityInstances->ClearInstances(); ObservationInstances->ClearInstances(); }
void AGeoPointLayerActor::OnMessageAccepted(const FGeoMessageEnvelope& Message) { Submit(Message); }
void AGeoPointLayerActor::OnLayerVisibilityChanged(const FString& LayerId, bool bVisible) { if (LayerId == TEXT("core.point-markers")) SetActorHiddenInGame(!bVisible); }
