#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Actor.h"
#include "IonAuroraActor.generated.h"

class UInstancedStaticMeshComponent;

UCLASS()
class IONCOMMANDVISUALIZATION_API AIonAuroraActor final : public AActor
{
    GENERATED_BODY()

public:
    AIonAuroraActor();
    virtual void OnConstruction(const FTransform& Transform) override;
    virtual void PostRegisterAllComponents() override;
    virtual void BeginPlay() override;
    virtual void Tick(float DeltaSeconds) override;

private:
    void BuildOval(UInstancedStaticMeshComponent* Instances, bool bNorth);
    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> NorthOval;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> SouthOval;
};
