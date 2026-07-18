#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Actor.h"
#include "IonCommandDeckActor.generated.h"

class UStaticMeshComponent;
class UTextRenderComponent;

UCLASS()
class IONCOMMANDUI_API AIonCommandDeckActor final : public AActor
{
    GENERATED_BODY()

public:
    AIonCommandDeckActor();
    virtual void BeginPlay() override;
    virtual void Tick(float DeltaSeconds) override;

private:
    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    UPROPERTY(VisibleAnywhere) TArray<TObjectPtr<UStaticMeshComponent>> ConsoleSurfaces;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UTextRenderComponent> TitleText;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UTextRenderComponent> StreamText;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UTextRenderComponent> TelemetryText;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UTextRenderComponent> SelectionText;
};
