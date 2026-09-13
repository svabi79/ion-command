#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Actor.h"
#include "GeoLayerTypes.h"
#include "GeoPathLayerActor.generated.h"

class UGeoDataSubsystem;
class UInstancedStaticMeshComponent;
class UMaterialInstanceDynamic;

struct FRenderedGeoPath
{
    FString EntityKey;
    FGeoMessageEnvelope Message;
    FLinearColor Color = FLinearColor(0.34f, 0.60f, 0.70f);
    int32 LegendIndex = 2;
    double LastSeenSeconds = 0.0;
    double ExpireAtSeconds = 0.0;
};

UCLASS()
class IONCOMMANDVISUALIZATION_API AGeoPathLayerActor final : public AActor, public IGeoRenderAdapter
{
    GENERATED_BODY()

public:
    AGeoPathLayerActor();
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
    FString GetLegendNote() const;

    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double GlobeRadius = 1000.0;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 MaxVisiblePaths = 800;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double PathLifetimeSeconds = 2592000.0;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double PathThickness = 0.014;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 MaxVerticesPerLine = 96;

private:
    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    void BuildEditorPreview();
    void OnLayerVisibilityChanged(const FString& LayerId, bool bVisible);
    FGeoLayerManifest CreateLayerManifest() const;
    FString ResolveEntityKey(const FGeoMessageEnvelope& Message) const;
    FLinearColor ResolveColor(const FGeoMessageEnvelope& Message) const;
    int32 ResolveLegendIndex(const FGeoMessageEnvelope& Message) const;
    bool IsExpired(const FRenderedGeoPath& Path, double NowSeconds) const;
    int32 EvictExpiredPaths(double NowSeconds, int32 MaxRemovals);
    int32 TrimToCapacity(int32 MaxRemovals);
    void RemovePathAt(int32 Index);
    void RebuildMeshes();
    void ProjectLine(const TArray<FGeoPosition>& Line, TArray<FVector>& OutWorld) const;
    void AppendSegmentTransforms(TArray<FTransform>& Out, const TArray<FVector>& WorldLine) const;
    void RefreshSelectionHighlight();

    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    UPROPERTY(VisibleAnywhere) TArray<TObjectPtr<UInstancedStaticMeshComponent>> ClassMeshes;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> SelectionMesh;
    UPROPERTY(Transient) TArray<TObjectPtr<UMaterialInstanceDynamic>> ClassMaterials;
    TWeakObjectPtr<UGeoDataSubsystem> DataSubsystem;
    TArray<FRenderedGeoPath> ActivePaths;
    TMap<FString, int32> EntityToPath;
    FString HighlightedMessageId;
    double LastExpiryCheck = 0.0;
    bool bNeedsRebuild = false;
    FGeoRenderLayerStatistics RuntimeStats;
};
