#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Actor.h"
#include "GeoLayerTypes.h"
#include "GeoAreaLayerActor.generated.h"

class UGeoDataSubsystem;
class UInstancedStaticMeshComponent;
class UMaterialInstanceDynamic;
class UProceduralMeshComponent;

struct FRenderedGeoArea
{
    FString EntityKey;
    FGeoMessageEnvelope Message;
    FLinearColor Color = FLinearColor(1.0f, 0.55f, 0.12f);
    float Opacity = 0.22f;
    double LastSeenSeconds = 0.0;
    double ExpireAtSeconds = 0.0;
};

UCLASS()
class IONCOMMANDVISUALIZATION_API AGeoAreaLayerActor final : public AActor, public IGeoRenderAdapter
{
    GENERATED_BODY()

public:
    AGeoAreaLayerActor();
    virtual void OnConstruction(const FTransform& Transform) override;
    virtual void PostRegisterAllComponents() override;
    virtual void BeginPlay() override;
    virtual void EndPlay(const EEndPlayReason::Type EndPlayReason) override;
    virtual void Tick(float DeltaSeconds) override;

    virtual bool Supports(const FGeoMessageEnvelope& Message) const override;
    virtual void Submit(const FGeoMessageEnvelope& Message) override;
    virtual void Reset() override;

    bool FindClosestMessageToRay(const FVector& RayOrigin, const FVector& RayDirection, double RayLength, double MaxDistance, FGeoMessageEnvelope& OutMessage) const;
    FGeoRenderLayerStatistics GetRenderStatistics() const;

    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double GlobeRadius = 1000.0;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 MaxVisibleAreas = 64;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double AreaLifetimeSeconds = 7200.0;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double OutlineThickness = 0.018;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 MaxRingVertices = 256;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 MaxFillTriangles = 4096;

private:
    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    void BuildEditorPreview();
    void OnLayerVisibilityChanged(const FString& LayerId, bool bVisible);
    FGeoLayerManifest CreateLayerManifest() const;
    FString ResolveEntityKey(const FGeoMessageEnvelope& Message) const;
    FLinearColor ResolveColor(const FGeoMessageEnvelope& Message) const;
    float ResolveOpacity(const FGeoMessageEnvelope& Message) const;
    bool IsExpired(const FRenderedGeoArea& Area, double NowSeconds) const;
    int32 EvictExpiredAreas(double NowSeconds, int32 MaxRemovals);
    int32 TrimToCapacity(int32 MaxRemovals);
    void RemoveAreaAt(int32 Index);
    void RebuildMeshes();
    void ProjectRing(const TArray<FGeoPosition>& Ring, double Radius, TArray<FGeoPosition>& OutUnwrapped, TArray<FVector>& OutWorld) const;
    void AppendOutlineTransforms(TArray<FTransform>& Out, const TArray<FVector>& WorldRing) const;
    void TessellateRing(const TArray<FGeoPosition>& Unwrapped, const TArray<FVector>& WorldRing, TArray<FVector>& OutVertices, TArray<int32>& OutTriangles) const;
    void RefreshSelectionHighlight();

    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UProceduralMeshComponent> FillMesh;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> OutlineMesh;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> SelectionMesh;
    UPROPERTY(Transient) TObjectPtr<UMaterialInstanceDynamic> FillMaterial;
    TWeakObjectPtr<UGeoDataSubsystem> DataSubsystem;
    TArray<FRenderedGeoArea> ActiveAreas;
    TMap<FString, int32> EntityToArea;
    FString HighlightedMessageId;
    double LastExpiryCheck = 0.0;
    bool bNeedsRebuild = false;
    FGeoRenderLayerStatistics RuntimeStats;
};
