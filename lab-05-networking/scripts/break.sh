#!/bin/bash

TEMPLATE="charts/shop/templates/networkpolicy.yaml"
BACKUP="/tmp/networkpolicy.yaml.bak"

[ ! -f "$BACKUP" ] && cp "$TEMPLATE" "$BACKUP"

FAULT=$((RANDOM % 4))

case $FAULT in
  0)
    echo "Fault 0: Щит Экспекто Патронум ослеп - совы не находят адресата (DNS blocked)"
    perl -pi -e 's/kubernetes.io\/metadata.name: kube-system/kubernetes.io\/metadata.name: blocked/' "$TEMPLATE"
    ;;
  1)
    echo "Fault 1: Клеймо Гриффиндора стёрто - стража не узнаёт своих (wrong api label)"
    perl -pi -e 's/app: api-server/app: ap1-server/' "$TEMPLATE"
    ;;
  2)
    echo "Fault 2: Совы летят не тем заклинанием - письма в Тайную комнату не доходят (DNS egress broken)"
    perl -pi -e 's/protocol: UDP/protocol: SCTP/' "$TEMPLATE"
    ;;
  3)
    echo "Fault 3: Василиск сменил адрес - вход в Тайную комнату по другому порту (port 5433)"
    perl -pi -e 's/port: 5432/port: 5433/g' "$TEMPLATE"
    ;;
esac

helm upgrade shop charts/shop -n shop
echo "Поломка применена. Найди её по Карте Мародеров, а не гаданием."
echo "Откат: cp $BACKUP $TEMPLATE && helm upgrade shop charts/shop -n shop"